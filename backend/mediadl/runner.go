package mediadl

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"time"
)

// Runner executes one external tool and reports each output line. Tests replace
// it with a fake, so no test starts yt-dlp or touches the network.
type Runner interface {
	Run(ctx context.Context, bin string, args []string, on_line func(line string)) error
}

// MAX_LINE_BYTES bounds one output line. A `yt-dlp -J` result is a single line
// and can be large, so the cap is generous; longer lines are dropped.
const MAX_LINE_BYTES = 64 << 20

// KILL_GRACE is how long a cancelled process may keep its output pipes open
// before they are closed. A killed yt-dlp can leave an ffmpeg child holding them.
const KILL_GRACE = 5 * time.Second

// exec_runner is the production Runner. It starts the executable directly, with
// no shell, and splits combined stdout and stderr into lines.
type exec_runner struct{}

// NewExecRunner returns the production runner.
func NewExecRunner() Runner {
	return exec_runner{}
}

func (exec_runner) Run(ctx context.Context, bin string, args []string, on_line func(line string)) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.WaitDelay = KILL_GRACE
	hide_console(cmd)
	splitter := &line_writer{on_line: on_line}
	// The same writer for stdout and stderr: exec serialises writes to it, so the
	// splitter needs no lock and both streams keep their relative order.
	cmd.Stdout = splitter
	cmd.Stderr = splitter
	err := cmd.Run()
	splitter.flush()
	return err
}

// line_writer splits written bytes on \n and \r and reports each complete line.
type line_writer struct {
	buf     bytes.Buffer
	on_line func(line string)
	dropped bool
}

func (w *line_writer) Write(p []byte) (int, error) {
	consumed := len(p)
	for len(p) > 0 {
		index := bytes.IndexAny(p, "\r\n")
		if index < 0 {
			w.append(p)
			break
		}
		w.append(p[:index])
		w.emit()
		p = p[index+1:]
	}
	return consumed, nil
}

func (w *line_writer) append(part []byte) {
	if w.dropped {
		return
	}
	if w.buf.Len()+len(part) > MAX_LINE_BYTES {
		w.dropped = true
		w.buf.Reset()
		return
	}
	w.buf.Write(part)
}

func (w *line_writer) emit() {
	if !w.dropped && w.buf.Len() > 0 && w.on_line != nil {
		w.on_line(string(bytes.TrimSpace(w.buf.Bytes())))
	}
	w.buf.Reset()
	w.dropped = false
}

func (w *line_writer) flush() {
	if w.buf.Len() > 0 {
		w.emit()
	}
}

var _ io.Writer = (*line_writer)(nil)
