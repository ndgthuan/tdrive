package mediadl

import (
	"strings"
	"testing"
)

const sample_probe_json = `{"title":" Launch clip ","uploader":"Studio","duration":125.9,"thumbnail":"https://img.example/t.jpg","formats":[
{"format_id":"sb0","ext":"mhtml","vcodec":"none","acodec":"none","format_note":"storyboard"},
{"format_id":"18","ext":"mp4","height":360,"vcodec":"avc1","acodec":"mp4a","filesize":1000,"format_note":"360p"},
{"format_id":"137","ext":"mp4","height":1080,"vcodec":"avc1","acodec":"none","filesize_approx":5000,"format_note":"1080p"},
{"format_id":"140","ext":"m4a","vcodec":"none","acodec":"mp4a","filesize":300,"format_note":"medium"},
{"format_id":"bad id;rm","ext":"mp4","vcodec":"avc1","acodec":"mp4a"}
]}`

func TestParseProbeJSONBuildsChoicesAndSortsTallestFirst(t *testing.T) {
	result, err := parse_probe_json([]byte(sample_probe_json))
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Launch clip" || result.Uploader != "Studio" || result.DurationS != 125 {
		t.Fatalf("metadata wrong: %+v", result)
	}
	if len(result.Formats) != 3 {
		t.Fatalf("expected 3 usable formats, got %d: %+v", len(result.Formats), result.Formats)
	}
	first := result.Formats[0]
	if first.ID != "137" || first.Choice != "137+ba" || first.HasAudio || !first.HasVideo || first.ApproxSize != 5000 {
		t.Fatalf("video-only format wrong: %+v", first)
	}
	if result.Formats[1].Choice != "18" || !result.Formats[1].HasAudio {
		t.Fatalf("muxed format wrong: %+v", result.Formats[1])
	}
	if len(result.Presets) != len(PRESETS) {
		t.Fatalf("presets missing")
	}
}

func TestParseProbeRejectsPlaylistsAndGarbage(t *testing.T) {
	if _, err := parse_probe_json([]byte(`{"_type":"playlist","title":"x"}`)); err == nil || !strings.Contains(err.Error(), "playlist") {
		t.Fatalf("playlist must be refused, got %v", err)
	}
	if _, err := parse_probe_json([]byte(`{not json`)); err == nil {
		t.Fatal("bad JSON must fail")
	}
}

func TestParseProbeOutputPrefersJSONAndReportsErrors(t *testing.T) {
	if _, err := parse_probe_output([]string{"ERROR: Unsupported URL"}); err == nil || err.Error() != "Unsupported URL" {
		t.Fatalf("error line not surfaced: %v", err)
	}
	result, err := parse_probe_output([]string{"ERROR: noise", sample_probe_json})
	if err != nil || result.Title != "Launch clip" {
		t.Fatalf("json line not used: %v %+v", err, result)
	}
}
