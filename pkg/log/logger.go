package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const (
	LevelTrace = zerolog.TraceLevel
	LevelDebug = zerolog.DebugLevel
	LevelInfo  = zerolog.InfoLevel
	LevelWarn  = zerolog.WarnLevel
	LevelError = zerolog.ErrorLevel
	LevelFatal = zerolog.FatalLevel
	LevelPanic = zerolog.PanicLevel
)

var (
	sensitivePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)password`),
		regexp.MustCompile(`(?i)passwd`),
		regexp.MustCompile(`(?i)secret`),
		regexp.MustCompile(`(?i)token`),
		regexp.MustCompile(`(?i)api[_-]?key`),
		regexp.MustCompile(`(?i)apikey`),
		regexp.MustCompile(`(?i)auth[_-]?key`),
		regexp.MustCompile(`(?i)private[_-]?key`),
		regexp.MustCompile(`(?i)access[_-]?token`),
		regexp.MustCompile(`(?i)refresh[_-]?token`),
		regexp.MustCompile(`(?i)bearer`),
		regexp.MustCompile(`(?i)credential`),
		regexp.MustCompile(`(?i)client[_-]?secret`),
		regexp.MustCompile(`(?i)master[_-]?key`),
		regexp.MustCompile(`(?i)encryption[_-]?key`),
		regexp.MustCompile(`(?i)signing[_-]?key`),
		regexp.MustCompile(`(?i)certificate[_-]?key`),
	}
	redactedValue = "***REDACTED***"
)

func isSensitiveField(field string) bool {
	fieldLower := strings.ToLower(field)
	for _, pattern := range sensitivePatterns {
		if pattern.MatchString(fieldLower) {
			return true
		}
	}
	return false
}

func redactIfSensitive(field, value string) string {
	if isSensitiveField(field) {
		return redactedValue
	}
	return value
}

type SensitiveString string

func (s SensitiveString) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedValue)
}

func (s SensitiveString) String() string {
	return redactedValue
}

func (s SensitiveString) Value() string {
	return string(s)
}

type Logger struct {
	logger zerolog.Logger
	mu     sync.RWMutex
}

type Config struct {
	Level      zerolog.Level
	TimeFormat string
	Output     io.Writer
	NoColor    bool
	FormatJSON bool
}

var (
	globalLogger *Logger
	once         sync.Once
)

func New(cfg Config) *Logger {
	zerolog.TimeFieldFormat = cfg.TimeFormat
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	var logger zerolog.Logger
	if cfg.FormatJSON {
		logger = zerolog.New(output).Level(cfg.Level).With().Timestamp().Logger()
	} else {
		output = zerolog.ConsoleWriter{
			Out:        output,
			NoColor:    cfg.NoColor,
			TimeFormat: cfg.TimeFormat,
		}
		logger = zerolog.New(output).Level(cfg.Level).With().Timestamp().Logger()
	}

	return &Logger{logger: logger}
}

func Default() *Logger {
	return New(Config{
		Level:      zerolog.InfoLevel,
		TimeFormat: time.RFC3339,
		Output:     os.Stdout,
		NoColor:    false,
		FormatJSON: false,
	})
}

func Global() *Logger {
	once.Do(func() {
		globalLogger = Default()
	})
	return globalLogger
}

func SetGlobalLogger(logger *Logger) {
	globalLogger = logger
}

func SetLevel(level zerolog.Level) {
	if globalLogger != nil {
		globalLogger.mu.Lock()
		defer globalLogger.mu.Unlock()
		globalLogger.logger = globalLogger.logger.Level(level)
	}
}

func GetLevel() zerolog.Level {
	if globalLogger != nil {
		return globalLogger.logger.GetLevel()
	}
	return zerolog.InfoLevel
}

func (l *Logger) Trace() *zerolog.Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.Trace()
}

func (l *Logger) Debug() *zerolog.Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.Debug()
}

func (l *Logger) Info() *zerolog.Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.Info()
}

func (l *Logger) Warn() *zerolog.Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.Warn()
}

func (l *Logger) Error() *zerolog.Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.Error()
}

func (l *Logger) Fatal() *zerolog.Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.Fatal()
}

func (l *Logger) Panic() *zerolog.Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.Panic()
}

func (l *Logger) WithStr(key, val string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	return &Logger{logger: l.logger.With().Str(key, val).Logger()}
}

func (l *Logger) WithErr(err error) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	return &Logger{logger: l.logger.With().Err(err).Logger()}
}

func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	ctx := l.logger.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return &Logger{logger: ctx.Logger()}
}

func (l *Logger) loggerWithContext(ctx context.Context) *zerolog.Logger {
	b := zerolog.Ctx(ctx)
	if b == nil {
		return &l.logger
	}
	return b
}

func (l *Logger) LoggerWithContext(ctx context.Context) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.loggerWithContext(ctx)
	return &Logger{logger: *b}
}

func FromContext(ctx context.Context) *Logger {
	b := zerolog.Ctx(ctx)
	if b == nil {
		return Global()
	}
	return &Logger{logger: *b}
}

type Field struct {
	Key   string
	Value interface{}
}

func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value}
}

func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

func Time(key string, value time.Time) Field {
	return Field{Key: key, Value: value}
}

func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value}
}

func Err(err error) Field {
	return Field{Key: "error", Value: err}
}

func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

func (l *Logger) InfoKV(event string, fields map[string]interface{}) {
	e := l.Info().Str("event", event)
	for k, v := range fields {
		e = e.Interface(k, v)
	}
	e.Msg("")
}

func (l *Logger) ErrorKV(event string, err error, fields map[string]interface{}) {
	e := l.Error().Str("event", event).Err(err)
	for k, v := range fields {
		e = e.Interface(k, v)
	}
	e.Msg("")
}

func (l *Logger) Metric(name string, value float64, tags map[string]string) {
	e := l.Info().
		Str("type", "metric").
		Str("metric_name", name).
		Float64("value", value)
	for k, v := range tags {
		e = e.Str(fmt.Sprintf("tag_%s", k), v)
	}
	e.Msg("")
}

func (l *Logger) Request(method, path string, duration time.Duration, statusCode int) {
	l.Info().
		Str("event", "request").
		Str("method", method).
		Str("path", path).
		Dur("duration", duration).
		Int("status_code", statusCode).
		Msg("")
}

func (l *Logger) Command(cmd string, args []string, duration time.Duration, err error) {
	e := l.Info().
		Str("event", "command").
		Str("command", cmd).
		Dur("duration", duration)

	if len(args) > 0 {
		e = e.Interface("args", args)
	}

	if err != nil {
		e = e.Err(err)
	}

	e.Msg("")
}

func (l *Logger) Output() io.Writer {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger
}

func (l *Logger) SetOutput(output io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger = l.logger.Output(output)
}

func (l *Logger) Close() error {
	return nil
}

type AuditEvent struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"`
	User      string                 `json:"user"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource"`
	Result    string                 `json:"result"`
	Details   map[string]interface{} `json:"details"`
	ClientIP  string                 `json:"client_ip"`
}

type AuditLogger struct {
	writer io.Writer
	logger *Logger
}

func NewAuditLogger(output io.Writer) *AuditLogger {
	if output == nil {
		output = os.Stdout
	}
	return &AuditLogger{
		writer: output,
		logger: Default(),
	}
}

func generateEventID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (al *AuditLogger) Log(event AuditEvent) error {
	event.ID = generateEventID()
	event.Timestamp = time.Now().UTC()

	event.Details = redactMapValues(event.Details)

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	_, err = al.writer.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}

	return nil
}

func redactMapValues(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = redactValue(k, v)
	}
	return result
}

func redactValue(key string, value interface{}) interface{} {
	if isSensitiveField(key) {
		return redactedValue
	}

	switch v := value.(type) {
	case string:
		return v
	case map[string]interface{}:
		return redactMapValues(v)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = redactValue(key, item)
		}
		return result
	default:
		return value
	}
}

func (al *AuditLogger) LogAuth(user, action, result string) error {
	return al.Log(AuditEvent{
		Type:   "auth",
		User:   user,
		Action: action,
		Result: result,
	})
}

func (al *AuditLogger) LogCommand(user, cmd string, result string) error {
	return al.Log(AuditEvent{
		Type:   "command",
		User:   user,
		Action: cmd,
		Result: result,
	})
}

func (al *AuditLogger) LogAccess(user, resource, result string) error {
	return al.Log(AuditEvent{
		Type:   "access",
		User:   user,
		Action: resource,
		Result: result,
	})
}

func (al *AuditLogger) LogConfigChange(user, field, oldValue, newValue string) error {
	return al.Log(AuditEvent{
		Type:   "config",
		User:   user,
		Action: fmt.Sprintf("update.%s", field),
		Details: map[string]interface{}{
			"field":     field,
			"old_value": redactIfSensitive(field, oldValue),
			"new_value": redactIfSensitive(field, newValue),
		},
	})
}

func (al *AuditLogger) Close() error {
	if closer, ok := al.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
