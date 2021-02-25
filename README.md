## unilog -- a logger process for daemontools [![Build Status](https://travis-ci.org/stripe/unilog.svg?branch=master)](https://travis-ci.org/stripe/unilog)

unilog is a tool for use with djb's [daemontools][daemontools]
process-monitor.

The way logging works with daemontools is that all output from a
daemontools-controlled process runs into a pipe, which is passed to a
second program, which can read from the pipe and format log output,
write it to disk or whatever. Daemontools ships with such a tool,
called ["multilog"][multilog], but it is arcane and weird, so we use
this one.

The job of unilog is to read lines of log output from stdin (which
will normally be connected to a running daemon by way of a pipe),
format them (which normally consists just of adding a timestamp), and
write them to a log file provided on the command-line.

If unilog receives a `SIGHUP` or `SIGALRM`, it responds by closing and
reopening the output file. This can be used to perform graceful log
rotation without requiring any special support from the running
daemon.

If unilog is unable to open or write to the output file, it will email
about this error, once per hour, until it succeeds in a write,
discarding output in the process.

In addition to the pipe buffer, unilog maintains an in-process buffer
of log lines that it will fill up if writes to the disk are slow or
blocking. This is intended to prevent the kernel pipe buffers from
filling up (and thus blocking the daemon from writing) during brief
periods of disk overload or hangs (sadly common in virtualized
environments).

### Filters

Unilog can be configured to apply filters to each line and perform arbitrary transformations. (For example, you may want to strip out sensitive information, or strip high-volume logs).

### Criticality and Austerity

Unilog contains an optional system for managing log volume, using criticality and austerity levels. If this systems is enabled, every log line has a **criticality level** associated with it. There are four levels of log criticality. In ascending order of importance, they are: `sheddable`, `sheddableplus` (default), `critical`, and `criticalplus`. (These names are taken from [Site Reliability Engineering, How Google Runs Production Systems](https://landing.google.com/sre/book.html).)

During times of high log volume, log lines may be sampled at exponential rates. The criticality level (clevel) of a log line determines its relative priority when sampling. By default, the system **austerity level** is set to `sheddable`, which means that all lines are preserved. If the austerity level is raised to `sheddableplus`, then only 10% of lines logged at `sheddable` are preserved, and the rest are filtered. If the austerity level is raised to `critical`, then 10% of lines logged at `clevel=sheddableplus` are preserved, and 1% of lines logged at `clevel=sheddable` are preserved, and so forth.

Criticality levels operate using filters, so this system is not just limited to sampling logs to reduce volume - it can be used to apply arbitrary transformations to a random subset of log lines.

[daemontools]: http://cr.yp.to/daemontools.html
[multilog]: http://cr.yp.to/daemontools/multilog.html
[googlsre]: https://landing.google.com/sre/book.html
