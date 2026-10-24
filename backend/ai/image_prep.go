// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package ai

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	// thumb_max_side bounds the longest edge of an image sent to a model.
	thumb_max_side = 512
	// max_source_pixels rejects decompression bombs before full decode.
	max_source_pixels  = 40 << 20
	thumb_jpeg_quality = 80
)

// prepare_image decodes encoded image bytes, downscales them so the longest
// edge is at most thumb_max_side, and re-encodes them as JPEG. Anything that is
// not a decodable image is an error, so the caller can skip it.
func prepare_image(raw []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode image config: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > max_source_pixels {
		return nil, fmt.Errorf("image size %dx%d is not supported", cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	scaled_width, scaled_height := width, height
	if width > thumb_max_side || height > thumb_max_side {
		if width >= height {
			scaled_width = thumb_max_side
			scaled_height = max(1, height*thumb_max_side/width)
		} else {
			scaled_height = thumb_max_side
			scaled_width = max(1, width*thumb_max_side/height)
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, scaled_width, scaled_height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: thumb_jpeg_quality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return out.Bytes(), nil
}
