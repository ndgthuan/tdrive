package applog

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// RecentLogLines is how many formatted log lines the in-memory buffer keeps for
// Copy diagnostics. The newest lines win.
const RecentLogLines = 300

// maxRecentLineBytes truncates one line so a single long attribute cannot take
// up most of the buffer.
const maxRecentLineBytes = 1024

// recentSensitiveKeys are matched as case-insensitive substrings against
// attribute keys for the in-memory buffer. It is stricter than the file
// handler: anything that names a hash, phone number or credential is dropped.
var recentSensitiveKeys = []string{
	"password", "passphrase", "masterkey", "master_key",
	"apihash", "api_hash", "apikey", "api_key",
	"authkey", "auth_key", "sessiondata", "session_data",
	"capability", "secret", "token",
	"hash", "phone",
}

// recentSecretText matches a secret-looking key followed by a value in free
// text, such as "access_hash=123" or "lock_token: abc", and masks the value.
var recentSecretText = regexp.MustCompile(`(?i)([a-z_]*(?:hash|token|secret|password|passphrase|phone|session_?data|api_?key|auth_?key)[a-z_]*)(\s*[=:]\s*)\S+`)

// recentBearer masks an Authorization style token in free text.
var recentBearer = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)

// recentPhoneDigits matches digit runs as long as phone numbers (11 to 15
// digits, optionally with a leading plus). Shorter runs such as Unix times and
// most Telegram ids are kept, since they are what the walk logs need.
var recentPhoneDigits = regexp.MustCompile(`\+?\d{11,15}`)

var (
	recentLines = newRecentBuffer(RecentLogLines)
	recentReady atomic.Bool
)

// recentBuffer is a fixed-size ring of formatted lines. It is safe for
// concurrent use: every access holds mu.
type recentBuffer struct {
	mu       sync.Mutex
	capacity int
	lines    []string
	// oldest is the index of the oldest line once the buffer has filled.
	oldest int
}

func newRecentBuffer(capacity int) *recentBuffer {
	return &recentBuffer{capacity: capacity, lines: make([]string, 0, capacity)}
}

func (b *recentBuffer) add(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lines) < b.capacity {
		b.lines = append(b.lines, line)
		return
	}
	b.lines[b.oldest] = line
	b.oldest = (b.oldest + 1) % b.capacity
}

// last returns the newest n lines, oldest first. n <= 0 returns every line.
func (b *recentBuffer) last(n int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	size := len(b.lines)
	if n <= 0 || n > size {
		n = size
	}
	out := make([]string, 0, n)
	for i := size - n; i < size; i++ {
		out = append(out, b.lines[(b.oldest+i)%size])
	}
	return out
}

// RecentLog returns the newest maxLines lines of the backend log, oldest first,
// joined with newlines. Lines are redacted before they are kept. It fails when
// Init never installed the buffer, so callers can say the log is unavailable.
func RecentLog(maxLines int) (string, error) {
	if !recentReady.Load() {
		return "", errors.New("backend log is not recording in this process")
	}
	if maxLines <= 0 || maxLines > RecentLogLines {
		maxLines = RecentLogLines
	}
	return strings.Join(recentLines.last(maxLines), "\n"), nil
}

// recentHandler records every record it handles into the buffer and then
// passes it on to the wrapped handler, so the file and stderr output are unchanged.
type recentHandler struct {
	inner  slog.Handler
	buffer *recentBuffer
	// prefix is the group path of the attributes, such as "walk.".
	prefix string
	// attrs holds the attributes from WithAttrs, already formatted as key=value.
	attrs []string
}

func newRecentHandler(inner slog.Handler, buffer *recentBuffer) *recentHandler {
	return &recentHandler{inner: inner, buffer: buffer}
}

func (h *recentHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *recentHandler) Handle(ctx context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Time.UTC().Format("2006-01-02T15:04:05.000Z07:00"))
	b.WriteByte(' ')
	b.WriteString(r.Level.String())
	b.WriteByte(' ')
	b.WriteString(scrubText(r.Message))
	for _, attr := range h.attrs {
		b.WriteByte(' ')
		b.WriteString(attr)
	}
	r.Attrs(func(attr slog.Attr) bool {
		writeRecentAttr(&b, h.prefix, attr)
		return true
	})
	h.buffer.add(truncateLine(b.String()))
	return h.inner.Handle(ctx, r)
}

func (h *recentHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	next := &recentHandler{
		inner:  h.inner.WithAttrs(attrs),
		buffer: h.buffer,
		prefix: h.prefix,
		attrs:  append([]string(nil), h.attrs...),
	}
	for _, attr := range attrs {
		var b strings.Builder
		writeRecentAttr(&b, h.prefix, attr)
		next.attrs = append(next.attrs, strings.TrimPrefix(b.String(), " "))
	}
	return next
}

func (h *recentHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &recentHandler{
		inner:  h.inner.WithGroup(name),
		buffer: h.buffer,
		prefix: h.prefix + name + ".",
		attrs:  append([]string(nil), h.attrs...),
	}
}

// writeRecentAttr appends " key=value" for one attribute, flattening groups
// and redacting the value.
func writeRecentAttr(b *strings.Builder, prefix string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		groupPrefix := prefix
		if attr.Key != "" {
			groupPrefix = prefix + attr.Key + "."
		}
		for _, member := range attr.Value.Group() {
			writeRecentAttr(b, groupPrefix, member)
		}
		return
	}
	key := prefix + attr.Key
	b.WriteByte(' ')
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(quoteIfNeeded(redactedValue(key, attr.Value)))
}

// redactedValue masks the whole value for a sensitive key, and scrubs free text
// from every other value.
func redactedValue(key string, value slog.Value) string {
	lower := strings.ToLower(key)
	for _, sensitive := range recentSensitiveKeys {
		if strings.Contains(lower, sensitive) {
			return "[redacted]"
		}
	}
	return scrubText(value.String())
}

// scrubText masks secret-looking assignments, bearer tokens and phone-number
// sized digit runs in free text.
func scrubText(text string) string {
	text = recentSecretText.ReplaceAllString(text, "$1$2[redacted]")
	text = recentBearer.ReplaceAllString(text, "Bearer [redacted]")
	return recentPhoneDigits.ReplaceAllString(text, "[number]")
}

func quoteIfNeeded(value string) string {
	if value == "" || strings.ContainsAny(value, " =\"\t\r\n") {
		return strconv.Quote(value)
	}
	return value
}

// truncateLine cuts a line to maxRecentLineBytes without splitting a rune.
func truncateLine(line string) string {
	if len(line) <= maxRecentLineBytes {
		return line
	}
	cut := maxRecentLineBytes
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + "..."
}
