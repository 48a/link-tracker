package agent

import (
	"strings"
	"testing"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

func TestProcessor(t *testing.T) {
	t.Parallel()

	cfg := Config{
		StopWords:       []string{"spam", "ads", "promo"},
		ExcludedAuthors: []string{"bot-user"},
		MinLength:       20,
		SumThreshold:    50,
	}
	processor := NewProcessor(cfg)

	t.Run("filter by stop word", func(t *testing.T) {
		t.Parallel()

		update := domain.RawUpdate{
			ID:          1,
			Description: "is a very long update which contains spam inside",
			Author:      "user",
		}
		if result := processor.Process(update); result != nil {
			t.Errorf("expected update to be filtered out due to stop word")
		}
	})

	t.Run("filter by excluded author", func(t *testing.T) {
		t.Parallel()

		update := domain.RawUpdate{
			ID:          2,
			Description: "is a valid update message without bad words",
			Author:      "bot-user",
		}
		if result := processor.Process(update); result != nil {
			t.Errorf("expected update to be filtered out due to excluded author")
		}
	})

	t.Run("filter by minimum length", func(t *testing.T) {
		t.Parallel()

		update := domain.RawUpdate{
			ID:          3,
			Description: "too short",
			Author:      "good-user",
		}
		if result := processor.Process(update); result != nil {
			t.Errorf("expected update to be filtered out due to length")
		}
	})

	t.Run("valid update passes filtration", func(t *testing.T) {
		t.Parallel()

		update := domain.RawUpdate{
			ID:          4,
			Description: "is a completely valid update that should pass",
			Author:      "good-user",
			TgChatIDs:   []int64{111, 222},
		}
		result := processor.Process(update)
		if result == nil {
			t.Fatalf("expected update to pass filtration")
		}
		if result.ID != update.ID || len(result.TgChatIDs) != 2 || result.Priority != "HIGH" {
			t.Errorf("invalid mapping of valid update")
		}
	})

	t.Run("summarize long text", func(t *testing.T) {
		t.Parallel()

		update := domain.RawUpdate{
			ID:          5,
			Description: strings.Repeat("A", 80) + " it must be truncated",
			Author:      "good-user",
		}
		result := processor.Process(update)
		if result == nil {
			t.Fatalf("expected update to pass filtration")
		}
		if len(result.Description) > cfg.SumThreshold+3 {
			t.Errorf("expected description to be truncated, got length %d", len(result.Description))
		}
		if result.Description[len(result.Description)-3:] != "..." {
			t.Errorf("expected description to end with '...'")
		}
	})

	t.Run("short text passed without summarization", func(t *testing.T) {
		t.Parallel()

		update := domain.RawUpdate{
			ID:          6,
			Description: "this description is valid",
			Author:      "good-user",
		}
		result := processor.Process(update)
		if result == nil {
			t.Fatalf("expected update to pass filtration")
		}
		if result.Description != update.Description {
			t.Errorf("expected description to be unchanged, got %q", result.Description)
		}
	})
}
