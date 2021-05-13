package log

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	SetupLogging(configFromEnv())
}

// Logging environment variables
const (
	// IPFS_* prefixed env vars kept for backwards compatibility
	// for this release. They will not be available in the next
	// release.
	//
	// GOLOG_* env vars take precedences over IPFS_* env vars.
	envIPFSLogging    = "IPFS_LOGGING"
	envIPFSLoggingFmt = "IPFS_LOGGING_FMT"

	envLogging    = "GOLOG_LOG_LEVEL"
	envLoggingFmt = "GOLOG_LOG_FMT"

	envLoggingFile = "GOLOG_FILE" // /path/to/file
	envLoggingURL  = "GOLOG_URL"  // url that will be processed by sink in the zap

	envLoggingOutput = "GOLOG_OUTPUT"     // possible values: stdout|stderr|file combine multiple values with '+'
	envLoggingLabels = "GOLOG_LOG_LABELS" // comma-separated key-value pairs, i.e. "app=example_app,dc=sjc-1"
)

type LogFormat int

const (
	ColorizedOutput LogFormat = iota
	PlaintextOutput
	JSONOutput
)

type Config struct {
	// Format overrides the format of the log output. Defaults to ColorizedOutput
	Format LogFormat

	// Level is the default minimum enabled logging level.
	Level LogLevel

	// SubsystemLevels are the default levels per-subsystem. When unspecified, defaults to Level.
	SubsystemLevels map[string]LogLevel

	// Sampling configs log sampling, disabled if nil.
	Sampling *zap.SamplingConfig

	// Stdout indicates whether logs should be written to stdout.
	Stdout bool

	// Stderr indicates whether logs should be written to stderr.
	Stderr bool

	// File is a path to a file that logs will be written to.
	File string

	// URL with schema supported by zap. Use zap.RegisterSink
	URL string

	// AutoStdout automatically enables stdout output if the current program is
	// run from a terminal, or ((File is not set or not correct) and (URL is not set)).
	// Defaults to false.
	AutoStdout bool

	// Labels is a set of key-values to apply to all loggers
	Labels map[string]string
}

// ErrNoSuchLogger is returned when the util pkg is asked for a non existant logger
var ErrNoSuchLogger = errors.New("Error: No such logger")

var loggerMutex sync.RWMutex // guards access to global logger state

// loggers is the set of loggers in the system
var loggers = make(map[string]*zap.SugaredLogger)
var levels = make(map[string]zap.AtomicLevel)

// primaryFormat is the format of the primary core used for logging
var primaryFormat LogFormat = ColorizedOutput

// defaultLevel is the default log level
var defaultLevel LogLevel = LevelError

// primaryCore is the primary logging core
var primaryCore zapcore.Core

// loggerCore is the base for all loggers created by this package
var loggerCore = &lockedMultiCore{}

// SetupLogging will initialize the logger backend and set the flags.
// TODO calling this in `init` pushes all configuration to env variables
// - move it out of `init`? then we need to change all the code (js-ipfs, go-ipfs) to call this explicitly
// - have it look for a config file? need to define what that is
func SetupLogging(cfg Config) {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()

	primaryFormat = cfg.Format
	defaultLevel = cfg.Level

	outputPaths := []string{}

	fileSet := false
	// check if we log to a file
	if len(cfg.File) > 0 {
		if path, err := normalizePath(cfg.File); err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve log path '%q', logging to %s\n", cfg.File, outputPaths)
		} else {
			fileSet = true
			outputPaths = append(outputPaths, path)
		}
	}

	urlSet := false
	if len(cfg.URL) > 0 {
		urlSet = true
		outputPaths = append(outputPaths, cfg.URL)
	}

	isTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	noFileOrURL := !(fileSet || urlSet)
	if cfg.Stdout || cfg.AutoStdout && (isTTY || noFileOrURL) {
		outputPaths = append(outputPaths, "stdout")
	}
	if cfg.Stderr {
		outputPaths = append(outputPaths, "stderr")
	}

	ws, _, err := zap.Open(outputPaths...)
	if err != nil {
		panic(fmt.Sprintf("unable to open logging output: %v", err))
	}

	newPrimaryCore := newCore(primaryFormat, ws, LevelDebug) // the main core needs to log everything.

	// Enable log sampling, see https://github.com/uber-go/zap/blob/7b21229fb3f063275f4f169f8a79ad30aa001c51/config.go#L217
	if scfg := cfg.Sampling; scfg != nil {
		var samplerOpts []zapcore.SamplerOption
		if scfg.Hook != nil {
			samplerOpts = append(samplerOpts, zapcore.SamplerHook(scfg.Hook))
		}
		newPrimaryCore = zapcore.NewSamplerWithOptions(
			newPrimaryCore,
			time.Second,
			scfg.Initial,
			scfg.Thereafter,
			samplerOpts...,
		)
	}

	for k, v := range cfg.Labels {
		newPrimaryCore = newPrimaryCore.With([]zap.Field{zap.String(k, v)})
	}

	if primaryCore != nil {
		loggerCore.ReplaceCore(primaryCore, newPrimaryCore)
	} else {
		loggerCore.AddCore(newPrimaryCore)
	}
	primaryCore = newPrimaryCore

	setAllLoggers(defaultLevel)

	for name, level := range cfg.SubsystemLevels {
		if leveler, ok := levels[name]; ok {
			leveler.SetLevel(zapcore.Level(level))
		} else {
			levels[name] = zap.NewAtomicLevelAt(zapcore.Level(level))
		}
	}
}

// SetDebugLogging calls SetAllLoggers with logging.DEBUG
func SetDebugLogging() {
	SetAllLoggers(LevelDebug)
}

// SetAllLoggers changes the logging level of all loggers to lvl
func SetAllLoggers(lvl LogLevel) {
	loggerMutex.RLock()
	defer loggerMutex.RUnlock()

	setAllLoggers(lvl)
}

func setAllLoggers(lvl LogLevel) {
	for _, l := range levels {
		l.SetLevel(zapcore.Level(lvl))
	}
}

// SetLogLevel changes the log level of a specific subsystem
// name=="*" changes all subsystems
func SetLogLevel(name, level string) error {
	lvl, err := LevelFromString(level)
	if err != nil {
		return err
	}

	// wildcard, change all
	if name == "*" {
		SetAllLoggers(lvl)
		return nil
	}

	loggerMutex.RLock()
	defer loggerMutex.RUnlock()

	// Check if we have a logger by that name
	if _, ok := levels[name]; !ok {
		return ErrNoSuchLogger
	}

	levels[name].SetLevel(zapcore.Level(lvl))

	return nil
}

// SetLogLevelRegex sets all loggers to level `l` that match expression `e`.
// An error is returned if `e` fails to compile.
func SetLogLevelRegex(e, l string) error {
	lvl, err := LevelFromString(l)
	if err != nil {
		return err
	}

	rem, err := regexp.Compile(e)
	if err != nil {
		return err
	}

	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	for name := range loggers {
		if rem.MatchString(name) {
			levels[name].SetLevel(zapcore.Level(lvl))
		}
	}
	return nil
}

// GetSubsystems returns a slice containing the
// names of the current loggers
func GetSubsystems() []string {
	loggerMutex.RLock()
	defer loggerMutex.RUnlock()
	subs := make([]string, 0, len(loggers))

	for k := range loggers {
		subs = append(subs, k)
	}
	return subs
}

func getLogger(name string) *zap.SugaredLogger {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	log, ok := loggers[name]
	if !ok {
		level, ok := levels[name]
		if !ok {
			level = zap.NewAtomicLevelAt(zapcore.Level(defaultLevel))
			levels[name] = level
		}
		log = zap.New(loggerCore).
			WithOptions(
				zap.IncreaseLevel(level),
				zap.AddCaller(),
			).
			Named(name).
			Sugar()

		loggers[name] = log
	}

	return log
}

// configFromEnv returns a Config with defaults populated using environment variables.
func configFromEnv() Config {
	cfg := Config{
		Format:          ColorizedOutput,
		Stderr:          true,
		Level:           LevelError,
		SubsystemLevels: map[string]LogLevel{},
		Labels:          map[string]string{},
	}

	format := os.Getenv(envLoggingFmt)
	if format == "" {
		format = os.Getenv(envIPFSLoggingFmt)
	}

	switch format {
	case "nocolor":
		cfg.Format = PlaintextOutput
	case "json":
		cfg.Format = JSONOutput
	}

	lvl := os.Getenv(envLogging)
	if lvl == "" {
		lvl = os.Getenv(envIPFSLogging)
	}
	if lvl != "" {
		for _, kvs := range strings.Split(lvl, ",") {
			kv := strings.SplitN(kvs, "=", 2)
			lvl, err := LevelFromString(kv[len(kv)-1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error setting log level %q: %s\n", kvs, err)
				continue
			}
			switch len(kv) {
			case 1:
				cfg.Level = lvl
			case 2:
				cfg.SubsystemLevels[kv[0]] = lvl
			}
		}
	}

	cfg.File = os.Getenv(envLoggingFile)
	// Disable stderr logging when a file is specified
	// https://github.com/ipfs/go-log/issues/83
	if cfg.File != "" {
		cfg.Stderr = false
	}

	cfg.URL = os.Getenv(envLoggingURL)
	output := os.Getenv(envLoggingOutput)
	outputOptions := strings.Split(output, "+")
	for _, opt := range outputOptions {
		switch opt {
		case "stdout":
			cfg.Stdout = true
		case "stderr":
			cfg.Stderr = true
		case "file":
			if cfg.File == "" {
				fmt.Fprint(os.Stderr, "please specify a GOLOG_FILE value to write to")
			}
		case "url":
			if cfg.URL == "" {
				fmt.Fprint(os.Stderr, "please specify a GOLOG_URL value to write to")
			}
		}
	}

	labels := os.Getenv(envLoggingLabels)
	if labels != "" {
		labelKVs := strings.Split(labels, ",")
		for _, label := range labelKVs {
			kv := strings.Split(label, "=")
			if len(kv) != 2 {
				fmt.Fprint(os.Stderr, "invalid label k=v: ", label)
				continue
			}
			cfg.Labels[kv[0]] = kv[1]
		}
	}

	return cfg
}
