package logger

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
)

func TestNew_LevelParsing(t *testing.T) {
	cases := []struct {
		input    string
		expected zerolog.Level
	}{
		{"debug", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"invalid", zerolog.InfoLevel}, // fallback
		{"", zerolog.InfoLevel},        // fallback — critical case
		{"trace", zerolog.TraceLevel},
		{"fatal", zerolog.FatalLevel},
	}
	for _, tc := range cases {
		t.Run("level="+tc.input, func(t *testing.T) {
			l := New(tc.input)
			if l.GetLevel() != tc.expected {
				t.Errorf("level = %v, want %v", l.GetLevel(), tc.expected)
			}
		})
	}
}

// Regression: empty string must not produce NoLevel.
func TestNew_EmptyLevelIsNotNoLevel(t *testing.T) {
	l := New("")
	if l.GetLevel() == zerolog.NoLevel {
		t.Fatal("empty level produced NoLevel — logs will be suppressed")
	}
}

// Regression: unknown level must not silently disable logging.
func TestNew_InvalidLevelFallsBack(t *testing.T) {
	l := New("nonsense")
	if l.GetLevel() == zerolog.NoLevel {
		t.Fatal("invalid level produced NoLevel")
	}
	if l.GetLevel() != zerolog.InfoLevel {
		t.Errorf("expected InfoLevel fallback, got %v", l.GetLevel())
	}
}

func TestNew_OutputsJSON(t *testing.T) {
	var buf bytes.Buffer
	l := zerolog.New(&buf).With().Timestamp().Logger()
	l.Info().Str("k", "v").Msg("hello")

	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output not JSON: %v — %s", err, buf.String())
	}
	if raw["message"] != "hello" {
		t.Errorf("message = %v", raw["message"])
	}
	if raw["k"] != "v" {
		t.Errorf("k = %v", raw["k"])
	}
}

// Verify level filtering actually works.
func TestNew_DebugSuppressedAtInfo(t *testing.T) {
	var buf bytes.Buffer
	l := zerolog.New(&buf).Level(zerolog.InfoLevel)
	l.Debug().Msg("should not appear")
	l.Info().Msg("should appear")

	out := buf.String()
	if bytes.Contains([]byte(out), []byte("should not appear")) {
		t.Error("debug log leaked at info level")
	}
	if !bytes.Contains([]byte(out), []byte("should appear")) {
		t.Error("info log missing at info level")
	}
}
