package agentdesk

import "testing"

func TestQuestion(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Question: keep the wait or skip it?\nOptions: (a) keep (b) skip", "keep the wait or skip it?"},
		{"Context first.\r\n\r\nquestion:   Which provider?  \r\nMore text", "Which provider?"},
		{"QUESTION: A or B? Then Question: later?", "A or B?"},
		{"No question line here, just a trailing one?", ""},
		{"Question: has no question mark", ""},
		{"", ""},
	} {
		if got := Question(tc.in); got != tc.want {
			t.Errorf("Question(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
