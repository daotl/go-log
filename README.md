# go-log

DAOT Labs' fork of [ipfs/go-log](https://github.com/ipfs/go-log).

[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](http://ipn.io)
[![](https://img.shields.io/badge/project-DAOT%20Labs-red.svg?style=flat-square)](http://github.com/daotl)
[![standard-readme compliant](https://img.shields.io/badge/standard--readme-OK-green.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme)
[![go.dev reference](https://godoc.org/github.com/daotl/go-log?status.svg)](https://pkg.go.dev/github.com/daotl/go-log)
[![CircleCI](https://img.shields.io/circleci/build/github/daotl/go-log?style=flat-square)](https://circleci.com/gh/daotl/go-log)

<!---[![Coverage Status](https://coveralls.io/repos/github/daotl/go-log/badge.svg?branch=master)](https://coveralls.io/github/daotl/go-log?branch=master)--->


go-log wraps [zap](https://github.com/uber-go/zap) to provide a logging facade. go-log manages logging
instances and allows for their levels to be controlled individually.

## Additional features of this fork

Added formats:
- `CompactOutput`: a compact format which looks like:
```
D[2021-05-13T17:49:52.413+0800]	example/log.go:52	for Debug
I[2021-05-13T17:49:52.413+0800]	example/log.go:52	for Info
W[2021-05-13T17:49:52.413+0800]	example/log.go:52	for Warn
E[2021-05-13T17:49:52.413+0800]	example/log.go:52	for Error
p[2021-05-13T17:49:52.413+0800]	example/log.go:52	for DPanic
P[2021-05-13T17:49:52.413+0800]	example/log.go:52	for Panic
F[2021-05-13T17:49:52.413+0800]	example/log.go:52	for Fatal
```
- `ColorizedCompactOutput`: same as `CompactOutput` but colorized

Added config options:
- `AutoColor`: automatically switches between formats and their colorized counterparts
  (`ColorizedOutput` <-> `PlaintextOutput`, `ColorizedCompactOutput` <-> `CompactOutput`),
  if the current program is run from a terminal it switches to the colorized version, vice versa
- `AutoStdout`: automatically enables stdout output if the current program is run from a terminal,
  or ((File is not set or not correct) and (URL is not set))
- `Sampling`: configs log sampling
- `Lumberjack`: configs log rolling using [Lumberjack](https://github.com/natefinch/lumberjack)

## Install

```sh
go get github.com/daotl/go-log
```

## Usage

Once the package is imported under the name `logging`, an instance of `EventLogger` can be created like so:

```go
var log = logging.Logger("subsystem name")
```

It can then be used to emit log messages in plain printf-style messages at seven standard levels:

Levels may be set for all loggers:

```go
lvl, err := logging.LevelFromString("error")
  if err != nil {
    panic(err)
  }
logging.SetAllLoggers(lvl)
```

or individually:

```go
lvl, err := logging.LevelFromString("error")
  if err != nil {
    panic(err)
  }
logging.SetLogLevel("foo", "info")
```

## Contribute

Feel free to join in. All welcome. Open an [issue](https://github.com/daotl/go-log/issues)!

## License

MIT
