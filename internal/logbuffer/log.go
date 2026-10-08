package logbuffer

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Entry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}
type Buffer struct {
	mu      sync.Mutex
	entries []Entry
}

func (b *Buffer) Entries() []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}
func (b *Buffer) add(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) == 200 {
		copy(b.entries, b.entries[1:])
		b.entries = b.entries[:199]
	}
	b.entries = append(b.entries, e)
}

type Handler struct {
	Buffer *Buffer
	Next   slog.Handler
}

func (h Handler) Enabled(ctx context.Context, l slog.Level) bool { return h.Next.Enabled(ctx, l) }
func (h Handler) Handle(ctx context.Context, r slog.Record) error {
	message := r.Message
	r.Attrs(func(a slog.Attr) bool { message += " · " + a.Key + "=" + a.Value.String(); return true })
	h.Buffer.add(Entry{Time: r.Time, Level: r.Level.String(), Message: message})
	return h.Next.Handle(ctx, r)
}
func (h Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.Next = h.Next.WithAttrs(attrs)
	return h
}
func (h Handler) WithGroup(name string) slog.Handler { h.Next = h.Next.WithGroup(name); return h }
