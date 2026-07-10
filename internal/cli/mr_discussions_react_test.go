package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const currentUserPath = "/api/v4/user"

func TestParseNoteID(t *testing.T) {
	id, err := parseNoteID("901")
	if err != nil || id != 901 {
		t.Fatalf("expected 901, got %d (err %v)", id, err)
	}

	for _, ref := range []string{"abc", "0", "-3", "9.5", ""} {
		t.Run(ref, func(t *testing.T) {
			_, err := parseNoteID(ref)
			if !errors.Is(err, errInvalidNoteID) {
				t.Fatalf("expected errInvalidNoteID, got %v", err)
			}
			if exitCodeForError(err) != 2 {
				t.Errorf("expected exit code 2, got %d", exitCodeForError(err))
			}
		})
	}
}

func TestNormalizeEmojiName(t *testing.T) {
	valid := map[string]string{
		"thumbsup":    "thumbsup",
		":thumbsup:":  "thumbsup",
		" :rocket: ":  "rocket",
		"heavy_plus1": "heavy_plus1",
	}
	for raw, want := range valid {
		t.Run(raw, func(t *testing.T) {
			got, err := normalizeEmojiName(raw)
			if err != nil || got != want {
				t.Fatalf("expected %q, got %q (err %v)", want, got, err)
			}
		})
	}

	for _, raw := range []string{"", "::", "th:umb", "two words", ":"} {
		t.Run("invalid "+raw, func(t *testing.T) {
			_, err := normalizeEmojiName(raw)
			if !errors.Is(err, errInvalidEmojiName) {
				t.Fatalf("expected errInvalidEmojiName, got %v", err)
			}
			if exitCodeForError(err) != 2 {
				t.Errorf("expected exit code 2, got %d", exitCodeForError(err))
			}
		})
	}
}

// aliceThreadJSON is the react-test fixture thread: notes 901 (alice) and
// 902 (bob), served on the full-ID discussion path.
func aliceThreadJSON() string {
	return discussionJSON(discussionIDUnresolvedAlice,
		discussionNoteJSON(901, "alice", "This branch check looks inverted", true, false, false, "2026-07-01T08:00:00Z", "2026-07-01T08:00:00Z"),
		discussionNoteJSON(902, "bob", "Agreed", true, false, false, "2026-07-02T09:00:00Z", "2026-07-02T09:00:00Z"),
	)
}

func TestMRDiscussionReactCreatesAward(t *testing.T) {
	var postCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDUnresolvedAlice:
			fmt.Fprint(w, aliceThreadJSON())
		case noteAwardPath(901):
			if r.Method != http.MethodPost {
				t.Errorf("expected award POST, got %s", r.Method)
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			postCount++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode award body: %v", err)
			}
			if got := body["name"]; got != "thumbsup" {
				t.Errorf("expected name=thumbsup body, got %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, awardJSON(55, "thumbsup", "me", 7))
		default:
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	// The colon form must normalize to the bare name before the API call.
	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "react", "123", discussionIDUnresolvedAlice, "901", ":thumbsup:")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if postCount != 1 {
		t.Fatalf("expected one POST, got %d", postCount)
	}
	for _, want := range []string{"reaction:", "discussion_id: 6f9a1c2d", "note_id: 901", "emoji: thumbsup", "action: react", "unreact 123 6f9a1c2d 901 thumbsup"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "noop") {
		t.Errorf("successful react must not be marked noop, got:\n%s", got)
	}
}

func TestMRDiscussionReactDuplicateIsVerifiedNoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDUnresolvedAlice:
			fmt.Fprint(w, aliceThreadJSON())
		case currentUserPath:
			fmt.Fprint(w, `{"id":7,"username":"me"}`)
		case noteAwardPath(901):
			switch r.Method {
			case http.MethodPost:
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"message":"404 Award Emoji Name has already been taken"}`)
			case http.MethodGet:
				fmt.Fprint(w, "["+awardJSON(55, "thumbsup", "me", 7)+"]")
			default:
				t.Errorf("unexpected method %s", r.Method)
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "react", "123", discussionIDUnresolvedAlice, "901", "thumbsup")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	for _, want := range []string{"action: react", "noop: true"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestMRDiscussionReactUnknownEmojiStaysError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDUnresolvedAlice:
			fmt.Fprint(w, aliceThreadJSON())
		case currentUserPath:
			fmt.Fprint(w, `{"id":7,"username":"me"}`)
		case noteAwardPath(901):
			switch r.Method {
			case http.MethodPost:
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"message":"404 Not Found"}`)
			case http.MethodGet:
				fmt.Fprint(w, "["+awardJSON(56, "thumbsup", "bob", 8)+"]")
			default:
				t.Errorf("unexpected method %s", r.Method)
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	_, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "react", "123", discussionIDUnresolvedAlice, "901", "notanemoji")
	if err == nil {
		t.Fatal("expected an error for a non-duplicate 404")
	}
	if exitCodeForError(err) != 1 {
		t.Errorf("expected exit code 1, got %d", exitCodeForError(err))
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	if !strings.Contains(out.String(), "code: gitlab_not_found") {
		t.Errorf("expected gitlab_not_found code, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "unknown emoji names") {
		t.Errorf("expected the emoji-name hint, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), server.URL) {
		t.Errorf("server URL must not leak into agent-facing errors:\n%s", out.String())
	}
}

func TestMRDiscussionUnreactDeletesOwnAward(t *testing.T) {
	var deletedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletedPaths = append(deletedPaths, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDUnresolvedAlice:
			fmt.Fprint(w, aliceThreadJSON())
		case currentUserPath:
			fmt.Fprint(w, `{"id":7,"username":"me"}`)
		case noteAwardPath(901):
			// The note carries several reactions: the caller's thumbsup and
			// rocket, plus bob's thumbsup.
			fmt.Fprint(w, "["+awardJSON(55, "thumbsup", "me", 7)+","+awardJSON(56, "thumbsup", "bob", 8)+","+awardJSON(57, "rocket", "me", 7)+"]")
		default:
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "unreact", "123", discussionIDUnresolvedAlice, "901", "thumbsup")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Only the caller's own award with the requested emoji may be deleted —
	// never another user's thumbsup, and never the caller's other emoji.
	if len(deletedPaths) != 1 || deletedPaths[0] != noteAwardPath(901)+"/55" {
		t.Fatalf("expected one DELETE of award 55, got %v", deletedPaths)
	}
	for _, want := range []string{"action: unreact", "react 123 6f9a1c2d 901 thumbsup"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "noop") {
		t.Errorf("successful unreact must not be marked noop, got:\n%s", got)
	}
}

func TestMRDiscussionUnreactMissingIsVerifiedNoop(t *testing.T) {
	var deleteCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCount++
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDUnresolvedAlice:
			fmt.Fprint(w, aliceThreadJSON())
		case currentUserPath:
			fmt.Fprint(w, `{"id":7,"username":"me"}`)
		case noteAwardPath(901):
			fmt.Fprint(w, "["+awardJSON(56, "thumbsup", "bob", 8)+"]")
		default:
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "unreact", "123", discussionIDUnresolvedAlice, "901", "thumbsup")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if deleteCount != 0 {
		t.Fatalf("verified no-op must not issue a DELETE, got %d", deleteCount)
	}
	for _, want := range []string{"action: unreact", "noop: true"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestMRDiscussionReactNoteNotInDiscussion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDUnresolvedAlice:
			fmt.Fprint(w, aliceThreadJSON())
		default:
			t.Errorf("membership guard must fail before any award call, got %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	_, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "react", "123", discussionIDUnresolvedAlice, "999", "thumbsup")
	if !errors.Is(err, errNoteNotInDiscussion) {
		t.Fatalf("expected errNoteNotInDiscussion, got %v", err)
	}
	if exitCodeForError(err) != 1 {
		t.Errorf("expected exit code 1, got %d", exitCodeForError(err))
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	if !strings.Contains(out.String(), "code: note_not_in_discussion") {
		t.Errorf("expected note_not_in_discussion code, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "901, 902") {
		t.Errorf("expected the thread's real note ids in the hint, got:\n%s", out.String())
	}
}

func TestMRDiscussionReactUsageErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		sentinel error
		code     string
	}{
		{name: "bad note id", args: []string{"react", "123", discussionIDUnresolvedAlice, "abc", "thumbsup"}, sentinel: errInvalidNoteID, code: "invalid_note_id"},
		{name: "bad emoji", args: []string{"react", "123", discussionIDUnresolvedAlice, "901", "th:umb"}, sentinel: errInvalidEmojiName, code: "invalid_emoji_name"},
		{name: "unreact bad emoji", args: []string{"unreact", "123", discussionIDUnresolvedAlice, "901", "::"}, sentinel: errInvalidEmojiName, code: "invalid_emoji_name"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"mr", "discussion"}, tc.args...)
			_, err := executeDiscussionCommand(t, commandModeAxi, "http://127.0.0.1:1", args...)
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("expected %v, got %v", tc.sentinel, err)
			}
			if exitCodeForError(err) != 2 {
				t.Errorf("expected exit code 2, got %d", exitCodeForError(err))
			}

			var out bytes.Buffer
			writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
			if !strings.Contains(out.String(), "code: "+tc.code) {
				t.Errorf("expected %s code, got:\n%s", tc.code, out.String())
			}
		})
	}

	t.Run("wrong arg count", func(t *testing.T) {
		_, err := executeDiscussionCommand(t, commandModeAxi, "http://127.0.0.1:1", "mr", "discussion", "react", "123", "6f9a", "901")
		if err == nil {
			t.Fatal("expected a usage error")
		}
		if exitCodeForError(err) != 2 {
			t.Errorf("expected exit code 2, got %d", exitCodeForError(err))
		}
	})
}

func TestMRDiscussionViewReactionFetchFailureFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDUnresolvedAlice:
			fmt.Fprint(w, aliceThreadJSON())
		case noteAwardPath(901), noteAwardPath(902):
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"boom"}`)
		default:
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	_, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "123", discussionIDUnresolvedAlice)
	if err == nil {
		t.Fatal("expected the view to fail loud on a reaction fetch error")
	}
	if !strings.Contains(err.Error(), "list reactions on note") {
		t.Errorf("expected reaction-fetch context in the error, got %v", err)
	}
}

func TestMRDiscussionsReactionsFlagAggregates(t *testing.T) {
	var awardHits int
	newServer := func(t *testing.T) *httptest.Server {
		t.Helper()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.EscapedPath() {
			case discussionsListPath:
				fmt.Fprint(w, discussionListStubJSON())
			case noteAwardPath(901):
				awardHits++
				fmt.Fprint(w, "["+awardJSON(55, "thumbsup", "alice", 7)+","+awardJSON(56, "thumbsup", "bob", 8)+"]")
			case noteAwardPath(902):
				awardHits++
				fmt.Fprint(w, "[]")
			case noteAwardPath(903):
				awardHits++
				fmt.Fprint(w, "["+awardJSON(57, "rocket", "mona", 9)+"]")
			default:
				t.Errorf("unexpected request path %s", r.URL.EscapedPath())
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		t.Cleanup(server.Close)

		return server
	}

	t.Run("with --reactions", func(t *testing.T) {
		awardHits = 0
		server := newServer(t)

		got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussions", "123", "--reactions")
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}

		// Default filters keep the two unresolved threads (notes 901+902, 903).
		if awardHits != 3 {
			t.Errorf("expected 3 award fetches for the page rows, got %d", awardHits)
		}
		for _, want := range []string{"reactions", "thumbsup:2", "rocket:1"} {
			if !strings.Contains(got, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("without --reactions", func(t *testing.T) {
		awardHits = 0
		server := newServer(t)

		got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussions", "123")
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}

		if awardHits != 0 {
			t.Errorf("expected zero award fetches without --reactions, got %d", awardHits)
		}
		if strings.Contains(got, "reactions") {
			t.Errorf("expected no reactions column by default, got:\n%s", got)
		}
	})
}

// mixedAwardsJSON is the multi-emoji, multi-user fixture for note 901:
// rocket ×3 (mona, alice, bob), thumbsup ×2 (bob, alice — alice awards two
// different emoji), heart ×1 (zed). Intentionally unsorted to prove the
// formatter orders deterministically.
func mixedAwardsJSON() string {
	return "[" + strings.Join([]string{
		awardJSON(60, "thumbsup", "bob", 8),
		awardJSON(61, "rocket", "mona", 9),
		awardJSON(62, "heart", "zed", 10),
		awardJSON(63, "rocket", "alice", 7),
		awardJSON(64, "thumbsup", "alice", 7),
		awardJSON(65, "rocket", "bob", 8),
	}, ",") + "]"
}

func mixedAwards(t *testing.T) []*gitlab.AwardEmoji {
	t.Helper()

	var awards []*gitlab.AwardEmoji
	if err := json.Unmarshal([]byte(mixedAwardsJSON()), &awards); err != nil {
		t.Fatalf("unmarshal mixed awards fixture: %v", err)
	}

	return awards
}

func TestFormatNoteReactions(t *testing.T) {
	t.Run("multiple emoji by multiple people", func(t *testing.T) {
		// Groups order by count desc, then name asc; usernames sort asc.
		want := "rocket:3(alice,bob,mona) thumbsup:2(alice,bob) heart:1(zed)"
		if got := output.FormatNoteReactions(mixedAwards(t)); got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("count ties break by name", func(t *testing.T) {
		var awards []*gitlab.AwardEmoji
		body := "[" + awardJSON(70, "thumbsup", "alice", 7) + "," + awardJSON(71, "rocket", "alice", 7) + "]"
		if err := json.Unmarshal([]byte(body), &awards); err != nil {
			t.Fatalf("unmarshal awards: %v", err)
		}
		want := "rocket:1(alice) thumbsup:1(alice)"
		if got := output.FormatNoteReactions(awards); got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("empty and nil entries", func(t *testing.T) {
		if got := output.FormatNoteReactions(nil); got != "" {
			t.Errorf("expected empty string for no awards, got %q", got)
		}
		if got := output.FormatNoteReactions([]*gitlab.AwardEmoji{nil}); got != "" {
			t.Errorf("expected nil entries to be skipped, got %q", got)
		}
	})
}

func TestAggregateReactions(t *testing.T) {
	want := "rocket:3 thumbsup:2 heart:1"
	if got := output.AggregateReactions(mixedAwards(t)); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}

	if got := output.AggregateReactions(nil); got != "" {
		t.Errorf("expected empty string for no awards, got %q", got)
	}
}

func TestMRDiscussionViewShowsMixedReactions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDUnresolvedAlice:
			fmt.Fprint(w, aliceThreadJSON())
		case noteAwardPath(901):
			fmt.Fprint(w, mixedAwardsJSON())
		case noteAwardPath(902):
			fmt.Fprint(w, "["+awardJSON(80, "eyes", "mona", 9)+"]")
		default:
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "123", discussionIDUnresolvedAlice)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Each note renders its own groups; reactions never bleed across notes.
	for _, want := range []string{
		"rocket:3(alice,bob,mona) thumbsup:2(alice,bob) heart:1(zed)",
		"eyes:1(mona)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected view to contain %q, got:\n%s", want, got)
		}
	}
}

func TestMRDiscussionReactOtherEmojiOrUserIsNotNoop(t *testing.T) {
	// GitLab's duplicate 404 must only become a no-op when the caller's own
	// award with the requested emoji exists — awards of the same emoji by
	// other users, or of other emoji by the caller, do not count.
	cases := []struct {
		name   string
		awards string
	}{
		{name: "own award has a different emoji", awards: "[" + awardJSON(55, "thumbsup", "me", 7) + "]"},
		{name: "same emoji awarded only by others", awards: "[" + awardJSON(56, "rocket", "bob", 8) + "," + awardJSON(57, "rocket", "mona", 9) + "]"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.EscapedPath() {
				case discussionsListPath + "/" + discussionIDUnresolvedAlice:
					fmt.Fprint(w, aliceThreadJSON())
				case currentUserPath:
					fmt.Fprint(w, `{"id":7,"username":"me"}`)
				case noteAwardPath(901):
					switch r.Method {
					case http.MethodPost:
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprint(w, `{"message":"404 Not Found"}`)
					case http.MethodGet:
						fmt.Fprint(w, tc.awards)
					default:
						t.Errorf("unexpected method %s", r.Method)
						w.WriteHeader(http.StatusMethodNotAllowed)
					}
				default:
					t.Errorf("unexpected request path %s", r.URL.EscapedPath())
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			_, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "react", "123", discussionIDUnresolvedAlice, "901", "rocket")
			if err == nil {
				t.Fatal("expected the 404 to stay an error, not a verified no-op")
			}
			if exitCodeForError(err) != 1 {
				t.Errorf("expected exit code 1, got %d", exitCodeForError(err))
			}
		})
	}
}

func TestMRDiscussionUnreactOnlyOtherUsersAwardsIsNoop(t *testing.T) {
	// Several people reacted with the requested emoji, but none of the awards
	// belong to the caller: verified no-op, and nobody's award is deleted.
	var deleteCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCount++
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDUnresolvedAlice:
			fmt.Fprint(w, aliceThreadJSON())
		case currentUserPath:
			fmt.Fprint(w, `{"id":7,"username":"me"}`)
		case noteAwardPath(901):
			fmt.Fprint(w, "["+awardJSON(56, "rocket", "bob", 8)+","+awardJSON(57, "rocket", "mona", 9)+","+awardJSON(58, "thumbsup", "bob", 8)+"]")
		default:
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "unreact", "123", discussionIDUnresolvedAlice, "901", "rocket")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if deleteCount != 0 {
		t.Fatalf("other users' awards must never be deleted, got %d DELETEs", deleteCount)
	}
	if !strings.Contains(got, "noop: true") {
		t.Errorf("expected a verified no-op, got:\n%s", got)
	}
}

func TestWriteDiscussionReactionStandardModes(t *testing.T) {
	out := output.DiscussionReactionOutput{
		DiscussionID: discussionIDUnresolvedAlice,
		NoteID:       901,
		Emoji:        "thumbsup",
	}

	t.Run("text react", func(t *testing.T) {
		var buf bytes.Buffer
		if err := output.WriteDiscussionReaction(&buf, "text", commandModeStandard, out, "react", false, 123, nil); err != nil {
			t.Fatalf("WriteDiscussionReaction returned error: %v", err)
		}
		for _, want := range []string{"reacted with :thumbsup: to note 901", "discussion: " + discussionIDUnresolvedAlice} {
			if !strings.Contains(buf.String(), want) {
				t.Errorf("expected text output to contain %q, got:\n%s", want, buf.String())
			}
		}
	})

	t.Run("text noop variants", func(t *testing.T) {
		var buf bytes.Buffer
		if err := output.WriteDiscussionReaction(&buf, "text", commandModeStandard, out, "react", true, 123, nil); err != nil {
			t.Fatalf("WriteDiscussionReaction returned error: %v", err)
		}
		if !strings.Contains(buf.String(), "already has your :thumbsup: reaction (no-op)") {
			t.Errorf("expected react noop text, got:\n%s", buf.String())
		}

		buf.Reset()
		if err := output.WriteDiscussionReaction(&buf, "text", commandModeStandard, out, "unreact", true, 123, nil); err != nil {
			t.Fatalf("WriteDiscussionReaction returned error: %v", err)
		}
		if !strings.Contains(buf.String(), "no :thumbsup: reaction from you (no-op)") {
			t.Errorf("expected unreact noop text, got:\n%s", buf.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		if err := output.WriteDiscussionReaction(&buf, "json", commandModeStandard, out, "react", false, 123, nil); err != nil {
			t.Fatalf("WriteDiscussionReaction returned error: %v", err)
		}
		for _, want := range []string{`"action": "react"`, `"note_id": 901`, fmt.Sprintf("%q", discussionIDUnresolvedAlice)} {
			if !strings.Contains(buf.String(), want) {
				t.Errorf("expected json output to contain %q, got:\n%s", want, buf.String())
			}
		}
	})
}
