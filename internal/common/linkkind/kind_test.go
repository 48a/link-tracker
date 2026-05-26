package linkkind

import "testing"

func TestKind(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		expected string
	}{
		{"gitHub https", "https://github.com/golang/go", "github"},
		{"gitHub http", "http://github.com/golang/go", "github"},
		{"gitHub with www", "https://www.github.com/golang/go", "github"},
		{"gitHub with dots and dashes", "https://github.com/my-org.name/my-repo.name", "github"},
		{"gitHub with subpaths", "https://github.com/golang/go/issues/1", "github"},
		{"gitHub mixed case", "HTTPS://GITHUB.COM/GOLANG/GO", "github"},

		{"gitHub missing repository", "https://github.com/golang", "none"},
		{"gitHub missing author and repository", "https://github.com/", "none"},
		{"gitHub empty author", "https://github.com//go", "none"},
		{"gitHub missing scheme", "github.com/golang/go", "none"},

		{"so HTTPS", "https://stackoverflow.com/questions/123456", "stackoverflow"},
		{"so HTTP", "http://stackoverflow.com/questions/123456", "stackoverflow"},
		{"so with www", "https://www.stackoverflow.com/questions/123456", "stackoverflow"},

		{"so with text after ID", "https://stackoverflow.com/questions/123456/abcde", "stackoverflow"},
		{"so missing question ID", "https://stackoverflow.com/questions/", "none"},
		{"so non-numeric ID", "https://stackoverflow.com/questions/abc", "none"},
		{"so missing questions path", "https://stackoverflow.com/123456", "none"},
		{"so missing scheme", "stackoverflow.com/questions/123456", "none"},
		{"empty string", "", "none"},
		{"random URL", "https://google.com", "none"},
		{"plain text", "not a url", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := Kind(tt.rawURL)
			if result != tt.expected {
				t.Errorf("Kind(%q) = %v; want %v", tt.rawURL, result, tt.expected)
			}
		})
	}
}
