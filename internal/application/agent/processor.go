package agent

import (
	"strings"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type Config struct {
	StopWords       []string
	ExcludedAuthors []string
	MinLength       int
	SumThreshold    int
}

type Processor struct {
	cfg Config
}

func NewProcessor(cfg Config) *Processor {
	return &Processor{cfg: cfg}
}

func (p *Processor) Process(update domain.RawUpdate) *domain.ProcessedUpdate {
	runes := []rune(update.Description)
	if len(runes) < p.cfg.MinLength {
		return nil
	}

	for _, author := range p.cfg.ExcludedAuthors {
		if update.Author == author {
			return nil
		}
	}

	lowerDesc := strings.ToLower(update.Description)
	for _, word := range p.cfg.StopWords {
		if strings.Contains(lowerDesc, strings.ToLower(word)) {
			return nil
		}
	}

	desc := update.Description
	if len(runes) > p.cfg.SumThreshold {
		desc = string(runes[:p.cfg.SumThreshold]) + "..."
	}

	return &domain.ProcessedUpdate{
		ID:          update.ID,
		Description: desc,
		TgChatIDs:   update.TgChatIDs,
		Priority:    "HIGH",
	}
}
