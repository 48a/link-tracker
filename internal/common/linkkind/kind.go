package linkkind

import "regexp"

var (
	githubRegex = regexp.MustCompile(`(?i)^https?://(?:www\.)?github\.com/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+`)
	soRegex     = regexp.MustCompile(`(?i)^https?://(?:www\.)?stackoverflow\.com/questions/\d+`)
)

func Kind(rawURL string) string {
	if githubRegex.MatchString(rawURL) {
		return "github"
	}
	if soRegex.MatchString(rawURL) {
		return "stackoverflow"
	}
	return "none"
}
