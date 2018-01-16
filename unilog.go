package unilog

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/getsentry/raven-go"

	"github.com/stripe/unilog/clevels"
	flag "launchpad.net/gnuflag"
)

// hold the argument passed in with "-statstags"
var statstags string

// A Filter is a function that takes in a log line and applies
// a transformation prior to prefixing them with a
// timestamp and logging them.
type Filter func(string) string

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

	Name        string
	Verbose     bool
	Debug       bool
	NoTimestamp bool

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
	boolFlag(&u.NoTimestamp, "notimestamp", "n", false, "Don't append timestamps to lines")
	flag.StringVar(&u.MailFrom, "mailfrom", u.MailFrom, "Address to send error emails from")
	flag.StringVar(&u.MailTo, "mailto", u.MailTo, "Address to send error emails to")
	flag.StringVar(&u.SentryDSN, "sentrydsn", u.SentryDSN, "Sentry DSN to send errors to")
	flag.StringVar(&u.StatsdAddress, "statsdaddress", "127.0.0.1:8200", "Address to send statsd metrics to")
	flag.StringVar(&clevels.AusterityFile, "austerityfile", clevels.AusterityFile, "(optional) Location of file to read austerity level from")
	stringFlag(&statstags, "statstags", "s", "", `(optional) tags to include with all statsd metrics (e.g. "foo:bar,baz:quz")`)
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
	Version = "0.3"
	// DefaultBuffer is the default size (in lines) of the
	// in-process line buffer
	DefaultBuffer = 1 << 12
)

// Stats is Unilog's statsd client.
var Stats *statsd.Client

func readlines(in io.Reader, bufsize int, shutdown chan struct{}) (<-chan string, <-chan error) {
	linec := make(chan string, bufsize)
	errc := make(chan error, 1)

	u := NewReader(in, shutdown)
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
					Stats.Count("unilog.bytes", int64(len(s)), nil, .1)
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
	return nil
}

func (u *Unilog) format(line string) string {
	for _, filter := range u.Filters {
		if filter != nil {
			line = filter(line)
		}
	}

	if u.NoTimestamp {
		return fmt.Sprintf("%s\n", line)
	} else {
		return fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05.000000"), line)
	}
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
		u.logLine(line)
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
		Stats.Count("unilog.errors_total", 1, []string{emsg}, 1)
	}

	if u.b.count == 0 && u.SentryDSN != "" {
		hostname, _ := os.Hostname()
		keys := map[string]string{
			"Hostname": hostname,
			"Action":   action,
			"Name":     u.Name,
			"Target":   u.target,
			"Error":    e.Error(),
			"Version":  Version,
		}
		raven.CaptureError(e, keys)
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
		fmt.Printf("This is unilog v%s\n", Version)
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

	u.lines, u.errs = readlines(os.Stdin, u.BufferLines, u.shutdown)

	fileName := u.target

	Stats, _ = statsd.New(u.StatsdAddress)

	Stats.Tags = append(Stats.Tags, fmt.Sprintf("file_name:%s", fileName))
	if statstags != "" {
		Stats.Tags = append(Stats.Tags, strings.Split(statstags, ",")...)
	}

	clevels.Stats = Stats

	_ = raven.SetDSN(u.SentryDSN)

	u.run()
}
