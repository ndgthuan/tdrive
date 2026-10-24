// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"context"
	"errors"
	"strings"
)

// UpdateYtDlp runs yt-dlp's own self-update on bin. It returns the last line
// yt-dlp printed, which states the outcome. A failed update returns an error
// carrying yt-dlp's message.
func UpdateYtDlp(ctx context.Context, runner Runner, bin string) (string, error) {
	var lines []string
	runErr := runner.Run(ctx, bin, []string{"--update"}, func(line string) {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimSpace(line))
		}
	})
	last := ""
	if len(lines) > 0 {
		last = lines[len(lines)-1]
	}
	if runErr != nil {
		return "", errors.New(first_nonempty(error_from_lines(lines), last, "yt-dlp could not update itself"))
	}
	return last, nil
}
