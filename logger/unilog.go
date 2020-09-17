package logger

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/getsentry/sentry-go"

	encjson "encoding/json"

	"github.com/stripe/unilog/clevels"
	"github.com/stripe/unilog/json"
	"github.com/stripe/unilog/reader"
	flag "launchpad.net/gnuflag"
)

// hold the argument passed in with "-statstags"
var statstags string

// hold the argument passed with "-cleveltags"
var cleveltags string

// hold the argument passed with "-independenttags"
var independenttags string

// Filter takes in a log line and applies a transformation prior to logging
// them. Since Unilog can operate on JSON or on string content, there are two
// methods that a filter must implement (so unilog can cut down on time spent
// parsing the log line).
type Filter interface {
	FilterLine(line string) string
	FilterJSON(line *json.LogLine)
}

// Unilog represents a unilog process. unilog is intended to be used
// as a standalone application, but is exported as a package to allow
// users to perform compile-time configuration to simplify deployment.
type Unilog struct {
	// Sentry DSN for reporting Unilog errors
	// If this is unset, unilog will not report errors to Sentry
	SentryDSN string
	// StatsdAddress for sending metrics
	// If this is unset, it wlil default to "127.0.0.1:8200" -> TODO: is this what we want?
	StatsdAddress string
	// The email address from which unilog will send mail on
	// errors
	MailTo string
	// The email address to which unilog will email breakages. If
	// either MailTo or MailFrom is unset, unilog will not
	// generate email.
	MailFrom string

	// A series of filters which will be applied to each log line
	// in order
	Filters []Filter

	// The version that unilog will report on the command-line and
	// in error emails. Defaults to the toplevel Version constant.
	Version string
	// The number of log lines to buffer in-memory, in case
	// unilog's disk writer falls behind. Note that when talking
	// to unilog over a pipe, the kernel also maintains an
	// in-kernel pipe buffer, sized 64kb on Linux.
	BufferLines int
	// Whether unilog expects log line input as JSON or as plain
	// text.
	JSON        bool
	jsonEncoder *encjson.Encoder

	Name    string
	Verbose bool
	Debug   bool

	lines     <-chan string
	errs      <-chan error
	sigReopen <-chan os.Signal
	sigTerm   <-chan os.Signal
	sigQuit   <-chan os.Signal
	shutdown  chan struct{}
	file      io.WriteCloser
	target    string

	b struct {
		broken bool
		at     time.Time
		count  int
	}

	exit           func(int)
	shouldShutdown bool
}

func stringFlag(val *string, longname, shortname, init, help string) {
	flag.StringVar(val, longname, init, help)
	flag.StringVar(val, shortname, init, help)
}

func boolFlag(val *bool, longname, shortname string, init bool, help string) {
	flag.BoolVar(val, longname, init, help)
	flag.BoolVar(val, shortname, init, help)
}

func (u *Unilog) fillDefaults() {
	u.exit = os.Exit
	if u.Version == "" {
		u.Version = Version
	}
	if u.BufferLines == 0 {
		u.BufferLines = DefaultBuffer
	}
}

func (u *Unilog) addFlags() {
	stringFlag(&u.Name, "name", "a", "", "Name of logged program")
	boolFlag(&u.Verbose, "verbose", "v", false, "Echo lines to stdout")
	boolFlag(&u.Debug, "debug", "d", false, "Print debug messages")
	flag.StringVar(&u.MailFrom, "mailfrom", u.MailFrom, "Address to send error emails from")
	flag.StringVar(&u.MailTo, "mailto", u.MailTo, "Address to send error emails to")
	flag.StringVar(&u.SentryDSN, "sentrydsn", u.SentryDSN, "Sentry DSN to send errors to")
	flag.StringVar(&u.StatsdAddress, "statsdaddress", "127.0.0.1:8200", "Address to send statsd metrics to")
	flag.StringVar(&clevels.AusterityFile, "austerityfile", clevels.AusterityFile, "(optional) Location of file to read austerity level from")
	stringFlag(&statstags, "statstags", "s", "", `(optional) tags to include with all statsd metrics except those about the box's austerity levels (format: "foo:bar,baz:quz")`)
	flag.StringVar(&independenttags, "independenttags", "", `(optional) tags to emit an independent metric for (format: "foo:bar,baz:quz" results in metrics "metricName.foo" and "metricName.baz")`)
	stringFlag(&cleveltags, "cleveltags", "", "", `(optional) tags to include with austerity statsd metrics. This applies to the "unilog.errors.load_level" and "unilog.austerity.box" metrics.`)
}

var emailTemplate = template.Must(template.New("email").Parse(`From: {{.From}}
To: {{.To}}
Subject: [unilog] {{.Name}} could not {{.Action}}

Hi there,

This is unilog reporting from {{.Hostname}}. I'm sad to report that
{{.Name}} is having some troubles writing to its log. I got caught up
trying to log a line to {{.Target}}.

To avoid spamming you, I'm going to shut up for an hour. Please fix me.

{{.Error}}
--
Sent from unilog {{.Version}}
`))

const (
	// Version is the Unilog version. Reported in emails and in
	// response to --version on the command line. Can be overriden
	// by the Version field in a Unilog object.
	Version = "1.0.1"
	// DefaultBuffer is the default size (in lines) of the
	// in-process line buffer
	DefaultBuffer = 1 << 12
)

var (
	// Commit vars can be set by passing e.g. -ldflags "$(TZ=UTC git --no-pager show --quiet --abbrev=12 --date='format-local:%Y-%m-%dT%H:%M:%SZ' --format="-X github.com/stripe/unilog/logger.commitDate=\"%cd\" -X github.com/stripe/unilog/logger.commitHash=%h")"
	commitHash = ""
	commitDate = ""
)

// SetCommitHash sets the commitHash contents, so dependents can inject build info
func SetCommitHash(hash string) {
	commitHash = hash
}

// SetCommitDate sets the commitDate contents, so dependents can inject build info
func SetCommitDate(date time.Time) {
	commitDate = date.Format("2006-01-02T15:04:05Z")
}

// Stats is Unilog's statsd client.
var Stats *statsd.Client

// tagPair is a simple pair of a tag t and the full metric name n
type tagPair struct {
	t string
	n string
}

// independentTags stores a list of tags to individually emit metrics on.
// Each metric name will be formatted exactly once and cached in metricsTable.
type independentTags struct {
	sync.RWMutex
	// The tags to build metric names from
	// Format is foo:bar where foo is the tag name
	Tags []string
	// Lookup table for metricName -> slice of metricName.tagName
	metricsTable map[string][]tagPair
}

func newIndependentTags(tags []string) *independentTags {
	return &independentTags{Tags: tags, metricsTable: make(map[string][]tagPair)}
}

func setupIndependentTags() *independentTags {
	return newIndependentTags(strings.Split(independenttags, ","))
}

func (it *independentTags) GetTags(metricName string) []tagPair {
	if it == nil {
		return []tagPair{}
	}
	it.Lock()
	defer it.Unlock()
	tags, ok := it.metricsTable[metricName]
	if ok {
		return tags
	}
	tags = make([]tagPair, 0, len(it.Tags))
	for _, tag := range it.Tags {
		prefix := strings.Split(tag, ":")[0]
		// If we don't get a token (e.g. we are passed the empty string), skip this tag
		if len(prefix) == 0 {
			continue
		}
		tags = append(tags, tagPair{t: tag, n: fmt.Sprintf("%s.%s", metricName, prefix)})
	}
	it.metricsTable[metricName] = tags
	return tags
}

// tagState holds the state necessary to efficiently emit independent metrics
var tagState *independentTags

// Client is the interface for our metrics client for use in independent metric emission
// Currently, only IndependentCount is implemented
type Client interface {
	Count(name string, value int64, tags []string, rate float64) error
}

// IndependentCount is a wrapper for the statsd.Count method. It will emit the normal metric
// in addition to a metric for each tag in independenttags with all global tags and that tag
// attached (along with tags passed as an argument to IndependentCount).
// Metric names will be of the form metricName.tag. Will short-circuit upon encountering an error.
func IndependentCount(client Client, name string, value int64, tags []string, rate float64) error {
	// Preserve backwards compatability by emitting the normal metric
	err := client.Count(name, value, tags, rate)
	if err != nil {
		return err
	}
	pairs := tagState.GetTags(name)

	// Emit independent metrics.
	for _, pair := range pairs {
		err = client.Count(pair.n, value, append(tags, pair.t), rate)
		if err != nil {
			return err
		}
	}
	return nil
}

func readlines(in io.Reader, bufsize int, shutdown chan struct{}) (<-chan string, <-chan error) {
	linec := make(chan string, bufsize)
	errc := make(chan error, 1)

	u := reader.NewReader(in, shutdown)
	r := bufio.NewReader(u)

	go func() {
		var err error
		var s string

		for err == nil {
			s, err = r.ReadString('\n')
			if s != "" {
				s = strings.TrimRight(s, "\n")
				linec <- s
				if Stats != nil {
					IndependentCount(Stats, "unilog.bytes", int64(len(s)), nil, .1)
				}
			}
		}

		if err != io.EOF {
			errc <- err
		}
		close(linec)
	}()

	return linec, errc
}

func (u *Unilog) reopen() error {
	if u.target == "-" {
		u.file = os.Stdout
		return nil
	}

	if u.file != nil {
		u.file.Close()
		u.file = nil
	}

	var e error
	if u.file, e = os.OpenFile(u.target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); e != nil {
		u.file = nil
		return e
	}

	if u.JSON {
		u.jsonEncoder = encjson.NewEncoder(u.file)
	}
	return nil
}

func (u *Unilog) format(line string) string {
	for _, filter := range u.Filters {
		if filter != nil {
			line = filter.FilterLine(line)
		}
	}
	return line + "\n"
}

func (u *Unilog) logLine(line string) {
	formatted := u.format(line)
	if u.Verbose {
		defer io.WriteString(os.Stdout, formatted)
	}

	var e error
	if u.file == nil {
		e = u.reopen()
	}
	if e != nil {
		u.handleError("reopen_file", e)
		return
	}
	_, e = io.WriteString(u.file, formatted)
	if e != nil {
		u.handleError("write_to_log", e)
	} else {
		u.b.broken = false
	}
}

func (u *Unilog) run() {
	for {
		if !u.tick() {
			return
		}
	}
}

func (u *Unilog) logJSON(jsonLine string) {
	var line json.LogLine
	err := encjson.Unmarshal(([]byte)(jsonLine), &line)
	if err != nil {
		// It won't parse, treat it as yolo text:
		u.logLine(jsonLine)
		return
	}

	if u.Verbose {
		defer fmt.Printf("%v\n", line)
	}
	for _, filter := range u.Filters {
		if filter != nil {
			filter.FilterJSON(&line)
		}
	}

	var e error
	if u.file == nil {
		e = u.reopen()
	}
	if e != nil {
		u.handleError("reopen_file", e)
		return
	}
	e = u.jsonEncoder.Encode(line)
	if e != nil {
		u.handleError("write_to_log", e)
	} else {
		u.b.broken = false
	}
}

// "tick" is Unilog's event loop
// returns true if Unilog should keep running,
// and false if it should stop.
func (u *Unilog) tick() bool {
	select {
	case e := <-u.errs:
		if e != nil && e != io.EOF {
			panic(e)
		} else {
			return false
		}
	case <-u.sigReopen:
		u.reopen()
	case <-u.sigTerm:
		select {
		case u.shutdown <- struct{}{}:
			u.shouldShutdown = true
		default:
		}
	case <-u.sigQuit:
		if u.shouldShutdown {
			u.exit(1)
			return false
		}
	case line, ok := <-u.lines:
		if !ok {
			return false
		}
		if !u.JSON {
			u.logLine(line)
		} else {
			u.logJSON(line)
		}
	}
	return true
}

func (u *Unilog) handleError(action string, e error) {
	if !u.b.broken {
		u.b.broken = true
		u.b.at = time.Now()
		u.b.count = 0
	} else if time.Since(u.b.at) > time.Hour {
		u.b.at = time.Now()
		u.b.count = 0
	}

	if u.Debug {
		fmt.Fprintf(os.Stderr, "Could not %s: %s\n", action, e.Error())
	}

	if Stats != nil {
		emsg := fmt.Sprintf("err_action:%s", action)
		IndependentCount(Stats, "unilog.errors_total", 1, []string{emsg}, 1)
	}

	if u.b.count == 0 && u.SentryDSN != "" {
		sentry.WithScope(func(scope *sentry.Scope) {
			hostname, _ := os.Hostname()
			scope.SetTags(map[string]string{
				"Hostname": hostname,
				"Action":   action,
				"Name":     u.Name,
				"Target":   u.target,
				"Error":    e.Error(),
				"Version":  Version,
			})
			sentry.CaptureException(e)
		})
	}

	if u.b.count == 0 && u.MailFrom != "" && u.MailTo != "" {
		message := new(bytes.Buffer)
		hostname, _ := os.Hostname()
		emailTemplate.Execute(message, map[string]string{
			"Hostname": hostname,
			"From":     u.MailFrom,
			"To":       u.MailTo,
			"Action":   action,
			"Name":     u.Name,
			"Target":   u.target,
			"Error":    e.Error(),
			"Version":  Version,
		})
		cmd := exec.Command("sendmail", "-t")
		cmd.Stdin = message
		cmd.Run()
	}

	u.b.count++
}

func setupStatsd(address, fileName, tags string) *statsd.Client {
	statsd, _ := statsd.New(address)

	if tags != "" {
		statsd.Tags = append(statsd.Tags, strings.Split(tags, ",")...)
	}
	return statsd
}

func (u *Unilog) setupSentry() {
	if u.SentryDSN != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn: u.SentryDSN,
		})

		if err != nil {
			// 2020-06-23, sentry-go 0.6.1: failure to parse the DSN is the only error case
			panic(fmt.Sprintf("Invalid DSN: %s", u.SentryDSN))
		}
	}
}

// Main sets up the Unilog instance and then calls Run.
func (u *Unilog) Main() {
	u.fillDefaults()

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] dstfile\n", os.Args[0])
		flag.PrintDefaults()
	}

	u.addFlags()
	var flagVersion bool
	boolFlag(&flagVersion, "version", "V", false, "Print the version number and exit")

	flag.Parse(true)

	if flagVersion {
		fmt.Printf("This is unilog v%s %s %s\n", Version, commitHash, commitDate)
		return
	}
	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	reopen := make(chan os.Signal, 2)
	signal.Notify(reopen, syscall.SIGALRM, syscall.SIGHUP)
	u.sigReopen = reopen

	term := make(chan os.Signal, 2)
	signal.Notify(term, syscall.SIGTERM, syscall.SIGINT)
	u.sigTerm = term

	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGQUIT)
	u.sigQuit = quit

	u.shutdown = make(chan struct{})
	u.target = flag.Arg(0)
	u.reopen()

	fileName := u.target

	tagState = setupIndependentTags()

	Stats = setupStatsd(u.StatsdAddress, fileName, statstags)

	clevels.Stats = setupStatsd(u.StatsdAddress, fileName, cleveltags)

	u.setupSentry()

	u.lines, u.errs = readlines(os.Stdin, u.BufferLines, u.shutdown)

	u.run()
}
