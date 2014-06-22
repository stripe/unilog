## unilog -- a logger process for daemontols [![Build Status](https://travis-ci.org/stripe/unilog.svg?branch=master)](https://travis-ci.org/stripe/unilog)

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

[daemontools]: http://cr.yp.to/daemontools.html
[multilog]: http://cr.yp.to/daemontools/multilog.html
