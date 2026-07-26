package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestInboxNavigation(t *testing.T) {
	m := newTestModel()

	m = pressKey(t, m, "j")
	if m.cursor != 1 {
		t.Fatalf("expected cursor to move down to 1, got %d", m.cursor)
	}

	m = pressKey(t, m, "k")
	if m.cursor != 0 {
		t.Fatalf("expected cursor to move up to 0, got %d", m.cursor)
	}
}

func TestOpenReaderAndBack(t *testing.T) {
	m := newTestModel()
	if !m.messages[0].Unread {
		t.Fatal("expected first fake message to start unread")
	}

	m = pressKey(t, m, "enter")
	if m.mode != readerView {
		t.Fatalf("expected reader view, got %v", m.mode)
	}
	if m.messages[0].Unread {
		t.Fatal("expected opening a message to mark it read")
	}

	m = pressKey(t, m, "b")
	if m.mode != inboxView {
		t.Fatalf("expected inbox view, got %v", m.mode)
	}
}

func TestOpenReaderLoadsBodyAndCachesIt(t *testing.T) {
	backend := &bodyBackend{body: "Hey Freddy,\n\nThe build passed."}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.messages[0].Body = ""
	m.messages[0].BodyLoaded = false
	m.loading = false

	next, cmd := m.Update(keyMsg("enter"))
	updated := next.(model)
	if updated.mode != readerView {
		t.Fatalf("expected reader view, got %v", updated.mode)
	}
	if cmd == nil {
		t.Fatal("expected opening an unloaded message to fetch its body")
	}
	if updated.status != "Loading email..." {
		t.Fatalf("expected loading status, got %q", updated.status)
	}

	loaded := cmd().(messageBodyLoadedMsg)
	next, _ = updated.Update(loaded)
	updated = next.(model)
	if !updated.messages[0].BodyLoaded {
		t.Fatal("expected message body to be cached")
	}
	if updated.messages[0].Body != "Hey Freddy,\n\nThe build passed." {
		t.Fatalf("unexpected loaded body: %q", updated.messages[0].Body)
	}
	if backend.reads != 1 {
		t.Fatalf("expected one backend read, got %d", backend.reads)
	}

	updated = pressKey(t, updated, "b")
	next, cmd = updated.Update(keyMsg("enter"))
	updated = next.(model)
	if cmd != nil {
		t.Fatal("expected cached body to reopen without another fetch")
	}
	if backend.reads != 1 {
		t.Fatalf("expected cached reopen to avoid backend read, got %d", backend.reads)
	}
}

func TestOpenReaderMarksMessageReadOnBackend(t *testing.T) {
	backend := &flagBackend{bodyBackend: bodyBackend{body: "Body"}}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.messages[0].Body = "Body"
	m.messages[0].BodyLoaded = true
	m.messages[0].Unread = true
	m.loading = false

	next, cmd := m.Update(keyMsg("enter"))
	updated := next.(model)
	if updated.messages[0].Unread {
		t.Fatal("expected opening to mark the message read locally")
	}
	if cmd == nil {
		t.Fatal("expected open to fire a mark-read command for an unread message")
	}
	if _, ok := cmd().(messageMarkedReadMsg); !ok {
		t.Fatalf("expected mark-read message, got %T", cmd())
	}
	if len(backend.markedRead) != 1 || backend.markedRead[0] != "1" {
		t.Fatalf("expected backend MarkMessageRead for id 1, got %v", backend.markedRead)
	}

	updated = pressKey(t, updated, "b")
	next, cmd = updated.Update(keyMsg("enter"))
	updated = next.(model)
	if cmd != nil {
		t.Fatal("expected reopen of an already-read message not to mark read again")
	}
	if len(backend.markedRead) != 1 {
		t.Fatalf("expected no second mark-read, got %v", backend.markedRead)
	}
}

func TestToggleReadUnreadFlipsStateAndCallsBackend(t *testing.T) {
	backend := &flagBackend{bodyBackend: bodyBackend{body: "Body"}}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.messages[0].Unread = false
	m.messages[0].BodyLoaded = true
	m.loading = false

	next, cmd := m.Update(keyMsg("m"))
	updated := next.(model)
	if !updated.messages[0].Unread {
		t.Fatal("expected toggle to mark read message as unread")
	}
	if cmd == nil {
		t.Fatal("expected toggle to fire a command")
	}
	cmd()
	if len(backend.markedUnread) != 1 || backend.markedUnread[0] != "1" {
		t.Fatalf("expected backend MarkMessageUnread for id 1, got %v", backend.markedUnread)
	}

	next, cmd = updated.Update(keyMsg("m"))
	updated = next.(model)
	if updated.messages[0].Unread {
		t.Fatal("expected second toggle to mark unread message as read")
	}
	if cmd == nil {
		t.Fatal("expected second toggle to fire a command")
	}
	cmd()
	if len(backend.markedRead) != 1 || backend.markedRead[0] != "1" {
		t.Fatalf("expected backend MarkMessageRead for id 1, got %v", backend.markedRead)
	}
}

func TestToggleFlaggedFlipsStateAndCallsBackend(t *testing.T) {
	backend := &flagBackend{bodyBackend: bodyBackend{body: "Body"}}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.messages[0].Flagged = false
	m.messages[0].BodyLoaded = true
	m.loading = false

	next, cmd := m.Update(keyMsg("s"))
	updated := next.(model)
	if !updated.messages[0].Flagged {
		t.Fatal("expected toggle to flag the message")
	}
	if cmd == nil {
		t.Fatal("expected toggle to fire a command")
	}
	cmd()
	if len(backend.flaggedIDs) != 1 || backend.flaggedIDs[0] != "1" {
		t.Fatalf("expected backend SetMessageFlagged(true) for id 1, got %v", backend.flaggedIDs)
	}

	next, cmd = updated.Update(keyMsg("s"))
	updated = next.(model)
	if updated.messages[0].Flagged {
		t.Fatal("expected second toggle to unflag the message")
	}
	if cmd == nil {
		t.Fatal("expected second toggle to fire a command")
	}
	cmd()
	if len(backend.unflaggedIDs) != 1 || backend.unflaggedIDs[0] != "1" {
		t.Fatalf("expected backend SetMessageFlagged(false) for id 1, got %v", backend.unflaggedIDs)
	}
}

func TestReaderScrollsBody(t *testing.T) {
	m := newTestModel()
	m.mode = readerView
	m.width = 80
	m.height = 12
	m.messages[0].Body = strings.Repeat("long line with enough words to wrap neatly\n", 40)
	m.messages[0].BodyLoaded = true

	m = pressKey(t, m, "j")
	if m.readerOffset != 1 {
		t.Fatalf("expected j to scroll reader to offset 1, got %d", m.readerOffset)
	}

	m = pressKey(t, m, "pgdown")
	if m.readerOffset <= 1 {
		t.Fatalf("expected pgdown to jump farther through the reader, got %d", m.readerOffset)
	}

	m = pressKey(t, m, "end")
	if m.readerOffset != m.maxReaderOffset() {
		t.Fatalf("expected end to jump to bottom, got %d of %d", m.readerOffset, m.maxReaderOffset())
	}

	m = pressKey(t, m, "home")
	if m.readerOffset != 0 {
		t.Fatalf("expected home to return to top, got %d", m.readerOffset)
	}
}

func TestHelpOverlayConsumesNavigation(t *testing.T) {
	m := newTestModel()

	m = pressKey(t, m, "?")
	if !m.showHelp {
		t.Fatal("expected help overlay to open")
	}

	m = pressKey(t, m, "j")
	if m.cursor != 0 {
		t.Fatalf("expected navigation to be ignored while help is open, got %d", m.cursor)
	}
	if !m.showHelp {
		t.Fatal("expected help overlay to remain open after ignored key")
	}

	m = pressKey(t, m, "q")
	if m.showHelp {
		t.Fatal("expected q to close help overlay")
	}
	if m.cursor != 0 {
		t.Fatalf("expected q in help overlay not to move cursor, got %d", m.cursor)
	}
}

func TestQuitFromInbox(t *testing.T) {
	m := newTestModel()

	_, cmd := m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("expected q from inbox to return a quit command")
	}
}

func TestSearchPromptCanCancel(t *testing.T) {
	m := newTestModel()

	m = pressKey(t, m, "/")
	if m.status == "" {
		t.Fatal("expected search action to show status")
	}

	m = pressKey(t, m, "esc")
	if m.searching {
		t.Fatal("expected esc to cancel search prompt")
	}
}

func TestArchiveRemovesSelectedMessage(t *testing.T) {
	backend := &actionFlowBackend{}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.loading = false

	next, cmd := m.Update(keyMsg("a"))
	updated := next.(model)
	if cmd == nil || !updated.action.Running {
		t.Fatal("expected archive to start a backend action")
	}
	done := cmd().(messageActionDoneMsg)
	next, _ = updated.Update(done)
	updated = next.(model)

	if backend.archived != "1" {
		t.Fatalf("expected first message to be archived, got %q", backend.archived)
	}
	if len(updated.messages) != 1 || updated.messages[0].ID != "2" {
		t.Fatalf("expected archived message to be removed, got %+v", updated.messages)
	}
	if !strings.Contains(updated.status, "Archived") {
		t.Fatalf("expected archived status, got %q", updated.status)
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	backend := &actionFlowBackend{}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.loading = false

	m = pressKey(t, m, "d")
	if !m.confirmDelete {
		t.Fatal("expected delete to ask for confirmation")
	}
	if backend.deleted != "" {
		t.Fatalf("expected delete not to run before confirmation, got %q", backend.deleted)
	}

	m = pressKey(t, m, "n")
	if m.confirmDelete {
		t.Fatal("expected n to cancel delete confirmation")
	}

	m = pressKey(t, m, "d")
	next, cmd := m.Update(keyMsg("y"))
	updated := next.(model)
	if cmd == nil || !updated.action.Running {
		t.Fatal("expected y to start delete action")
	}
	done := cmd().(messageActionDoneMsg)
	next, _ = updated.Update(done)
	updated = next.(model)

	if backend.deleted != "1" {
		t.Fatalf("expected first message to be deleted, got %q", backend.deleted)
	}
	if len(updated.messages) != 1 || updated.messages[0].ID != "2" {
		t.Fatalf("expected deleted message to be removed, got %+v", updated.messages)
	}
}

func TestDeleteCanSkipConfirmation(t *testing.T) {
	backend := &actionFlowBackend{}
	confirmDelete := false
	m := NewWithOptions(Options{backend: backend, ConfirmDelete: &confirmDelete})
	m.messages = testMessages()
	m.loading = false

	next, cmd := m.Update(keyMsg("d"))
	updated := next.(model)
	if updated.confirmDelete {
		t.Fatal("expected delete confirmation to be skipped")
	}
	if cmd == nil || !updated.action.Running {
		t.Fatal("expected delete to start immediately")
	}
}

func TestConfiguredEditorLabelWins(t *testing.T) {
	m := NewWithOptions(Options{Editor: "nano"})
	if got := m.editorLabel(); got != "nano" {
		t.Fatalf("expected configured editor, got %q", got)
	}
}

func TestSearchPromptLoadsSearchResults(t *testing.T) {
	backend := &actionFlowBackend{
		searchMessages: []message{{ID: "9", From: "Alice", Subject: "Deploy"}},
	}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.loading = false

	m = pressKey(t, m, "/")
	if !m.searching {
		t.Fatal("expected / to open search prompt")
	}
	m = pressKey(t, m, "Alice deploy")
	next, cmd := m.Update(keyMsg("enter"))
	updated := next.(model)
	if updated.searching {
		t.Fatal("expected enter to close search prompt")
	}
	if updated.searchQuery != "Alice deploy" {
		t.Fatalf("expected search query to be stored, got %q", updated.searchQuery)
	}
	if cmd == nil {
		t.Fatal("expected search to load inbox")
	}
	loaded := cmd().(inboxPageLoadedMsg)
	next, _ = updated.Update(loaded)
	updated = next.(model)

	if backend.searchQuery != "Alice deploy" {
		t.Fatalf("expected backend search query, got %q", backend.searchQuery)
	}
	if len(updated.messages) != 1 || updated.messages[0].ID != "9" {
		t.Fatalf("expected search results to replace inbox, got %+v", updated.messages)
	}
}

func TestEscClearsActiveSearch(t *testing.T) {
	backend := &actionFlowBackend{}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.searchQuery = "alice"
	m.loading = false

	next, cmd := m.Update(keyMsg("esc"))
	updated := next.(model)
	if updated.searchQuery != "" {
		t.Fatalf("expected search query to clear, got %q", updated.searchQuery)
	}
	if cmd == nil {
		t.Fatal("expected clearing search to reload inbox")
	}
}

func TestMailboxRailCanOpenUnreadView(t *testing.T) {
	backend := &mailboxSwitchTestBackend{
		mailbox: "INBOX",
		pages: map[string][][]message{
			"INBOX": {{
				{ID: "1", From: "Alice", Subject: "Unread", Unread: true},
				{ID: "2", From: "Bob", Subject: "Read", Unread: false},
			}},
		},
	}
	m := NewWithOptions(Options{backend: backend})
	m.width = 120
	m.height = 24
	m.messages = []message{
		{ID: "1", From: "Alice", Subject: "Unread", Unread: true},
		{ID: "2", From: "Bob", Subject: "Read", Unread: false},
	}
	m.loading = false

	m = pressKey(t, m, "tab")
	if !m.mailboxFocused {
		t.Fatal("expected tab to focus mailbox rail")
	}
	m = pressKey(t, m, "tab")
	if m.mailboxCursor != 1 {
		t.Fatalf("expected mailbox cursor on Unread, got %d", m.mailboxCursor)
	}

	next, cmd := m.Update(keyMsg("enter"))
	updated := next.(model)
	if cmd == nil {
		t.Fatal("expected unread view to reload")
	}
	if updated.mailboxFocused {
		t.Fatal("expected opening unread to return focus to messages")
	}
	if updated.mailboxFilter != unreadMailFilter {
		t.Fatalf("expected unread filter, got %v", updated.mailboxFilter)
	}

	loaded := cmd().(inboxPageLoadedMsg)
	next, _ = updated.Update(loaded)
	updated = next.(model)
	if len(updated.messages) != 1 || updated.messages[0].ID != "1" {
		t.Fatalf("expected only unread message, got %+v", updated.messages)
	}

	updated = pressKey(t, updated, "enter")
	if updated.mode != readerView {
		t.Fatalf("expected unread message to open, got %v", updated.mode)
	}
	updated = pressKey(t, updated, "b")
	if len(updated.messages) != 0 {
		t.Fatalf("expected read message to leave unread view, got %+v", updated.messages)
	}
}

func TestMailboxRailCanSwitchToSent(t *testing.T) {
	backend := &mailboxSwitchTestBackend{
		mailbox: "INBOX",
		pages: map[string][][]message{
			"INBOX": {{
				{ID: "1", From: "Alice", Subject: "Inbox"},
			}},
			"Sent": {{
				{ID: "9", From: "Me", Subject: "Sent message"},
			}},
		},
	}
	m := NewWithOptions(Options{backend: backend})
	m.width = 120
	m.height = 24
	m.messages = []message{{ID: "1", From: "Alice", Subject: "Inbox"}}
	m.loading = false

	m = pressKey(t, m, "tab")
	m = pressKey(t, m, "j")
	m = pressKey(t, m, "j")
	m = pressKey(t, m, "j")

	next, cmd := m.Update(keyMsg("enter"))
	updated := next.(model)
	if cmd == nil {
		t.Fatal("expected sent mailbox to reload")
	}
	if backend.mailbox != "Sent" || updated.mailbox != "Sent" {
		t.Fatalf("expected backend/model mailbox Sent, got backend=%q model=%q", backend.mailbox, updated.mailbox)
	}
	if updated.mailboxFilter != allMailFilter {
		t.Fatalf("expected all mail filter, got %v", updated.mailboxFilter)
	}

	loaded := cmd().(inboxPageLoadedMsg)
	next, _ = updated.Update(loaded)
	updated = next.(model)
	if len(updated.messages) != 1 || updated.messages[0].ID != "9" {
		t.Fatalf("expected sent message, got %+v", updated.messages)
	}
}

func TestComposeDraftFlowReviewsAndSends(t *testing.T) {
	backend := &draftFlowBackend{
		draft: "From: Freddy <freddy@example.com>\nTo: \nSubject: \n\n",
	}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.loading = false

	next, cmd := m.Update(keyMsg("c"))
	updated := next.(model)
	if cmd == nil {
		t.Fatal("expected compose to prepare a draft")
	}
	prepared := cmd().(draftPreparedMsg)
	defer removeDraftFile(prepared.path)

	next, editorCmd := updated.Update(prepared)
	updated = next.(model)
	if editorCmd != nil {
		t.Fatal("expected prepared draft to stay in the TUI")
	}
	if updated.draft.Path == "" {
		t.Fatal("expected draft path to be stored")
	}
	if updated.mode != draftReviewView {
		t.Fatalf("expected draft view, got %v", updated.mode)
	}
	if updated.draft.Focus != draftFieldTo {
		t.Fatalf("expected compose to focus To, got %v", updated.draft.Focus)
	}

	updated = pressKey(t, updated, "alice@example.com")
	updated = pressKey(t, updated, "tab")
	updated = pressKey(t, updated, "Hello")
	updated = pressKey(t, updated, "tab")
	updated = pressKey(t, updated, "Hi Alice.")
	if updated.draft.Summary.To != "alice@example.com" || updated.draft.Summary.Subject != "Hello" || updated.draft.Summary.Body != "Hi Alice." {
		t.Fatalf("expected TUI-edited draft summary, got %+v", updated.draft.Summary)
	}

	next, cmd = updated.Update(keyMsg("ctrl+s"))
	updated = next.(model)
	if cmd == nil || !updated.draft.Sending {
		t.Fatal("expected Ctrl+S to start sending")
	}

	sent := cmd().(draftSentMsg)
	next, _ = updated.Update(sent)
	updated = next.(model)

	want := "From: Freddy <freddy@example.com>\nTo: alice@example.com\nSubject: Hello\n\nHi Alice.\n"
	if backend.sent != want {
		t.Fatalf("expected TUI-edited draft to be sent, got %q", backend.sent)
	}
	if updated.mode != inboxView {
		t.Fatalf("expected compose send to return to inbox, got %v", updated.mode)
	}
	if _, err := os.Stat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("expected sent draft tempfile to be removed, stat err: %v", err)
	}
}

func TestReplyDraftFlowUsesSelectedMessage(t *testing.T) {
	backend := &draftFlowBackend{
		draft: "From: Freddy <freddy@example.com>\nTo: Alice <alice@example.com>\nSubject: Re: Design notes\n\n> old body\n",
	}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.mode = readerView
	m.loading = false

	next, cmd := m.Update(keyMsg("r"))
	updated := next.(model)
	if cmd == nil {
		t.Fatal("expected reply to prepare a draft")
	}
	prepared := cmd().(draftPreparedMsg)
	defer removeDraftFile(prepared.path)

	if backend.request.Kind != replyDraft {
		t.Fatalf("expected reply draft request, got %v", backend.request.Kind)
	}
	if backend.request.Message.ID != "1" || backend.request.Message.Email != "alice@example.com" {
		t.Fatalf("expected selected message in reply request, got %+v", backend.request.Message)
	}

	next, _ = updated.Update(prepared)
	updated = next.(model)
	if updated.mode != draftReviewView {
		t.Fatalf("expected draft review view, got %v", updated.mode)
	}
	if updated.draft.Summary.To != "Alice <alice@example.com>" {
		t.Fatalf("expected reply recipient in summary, got %+v", updated.draft.Summary)
	}
	if updated.draft.Focus != draftFieldBody {
		t.Fatalf("expected reply to focus body, got %v", updated.draft.Focus)
	}
}

func TestForwardDraftFlowUsesSelectedMessage(t *testing.T) {
	backend := &draftFlowBackend{
		draft: "From: Freddy <freddy@example.com>\nTo: \nSubject: Fwd: Design notes\n\n--------- Forwarded message ---------\nFrom: Alice\n\nold body\n",
	}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.mode = readerView
	m.loading = false

	next, cmd := m.Update(keyMsg("f"))
	updated := next.(model)
	if cmd == nil {
		t.Fatal("expected forward to prepare a draft")
	}
	prepared := cmd().(draftPreparedMsg)
	defer removeDraftFile(prepared.path)

	if backend.request.Kind != forwardDraft {
		t.Fatalf("expected forward draft request, got %v", backend.request.Kind)
	}
	if backend.request.Message.ID != "1" {
		t.Fatalf("expected selected message in forward request, got %+v", backend.request.Message)
	}

	next, _ = updated.Update(prepared)
	updated = next.(model)
	if updated.mode != draftReviewView {
		t.Fatalf("expected draft review view, got %v", updated.mode)
	}
	if updated.draft.Focus != draftFieldTo {
		t.Fatalf("expected forward to focus To field, got %v", updated.draft.Focus)
	}
	if !strings.Contains(updated.draft.Summary.Subject, "Fwd:") {
		t.Fatalf("expected Fwd: subject in summary, got %+v", updated.draft.Summary)
	}
	if !strings.Contains(updated.draft.Summary.Body, "Forwarded message") {
		t.Fatalf("expected forwarded body in summary, got %+v", updated.draft.Summary)
	}
}

func TestForwardRequiresReaderView(t *testing.T) {
	backend := &draftFlowBackend{}
	m := NewWithOptions(Options{backend: backend})
	m.messages = testMessages()
	m.mode = inboxView
	m.loading = false

	next, cmd := m.Update(keyMsg("f"))
	updated := next.(model)
	if cmd != nil {
		t.Fatal("expected forward from inbox to produce no command")
	}
	if updated.mode != inboxView {
		t.Fatalf("expected to stay in inbox view, got %v", updated.mode)
	}
	if backend.request.Kind == forwardDraft {
		t.Fatal("expected no draft request when forwarding from inbox")
	}
}

func TestDraftReviewRequiresRecipientBeforeSending(t *testing.T) {
	backend := &draftFlowBackend{}
	m := NewWithOptions(Options{backend: backend})
	m.mode = draftReviewView
	m.draft = draftState{
		Kind:    composeDraft,
		Content: "From: Freddy <freddy@example.com>\nSubject: Missing To\n\nHi.\n",
		Summary: parseDraftSummary("From: Freddy <freddy@example.com>\nSubject: Missing To\n\nHi.\n"),
		Serial:  1,
	}
	m.draftSerial = 1

	next, cmd := m.Update(keyMsg("ctrl+s"))
	updated := next.(model)
	if cmd != nil {
		t.Fatal("expected missing recipient to block send command")
	}
	if !strings.Contains(updated.status, "recipient") {
		t.Fatalf("expected recipient validation status, got %q", updated.status)
	}
	if backend.sent != "" {
		t.Fatalf("expected backend not to send, got %q", backend.sent)
	}
}

func TestEditorErrorWithDraftStillOpensReview(t *testing.T) {
	path, err := writeDraftFile("To: alice@example.com\nSubject: Saved\n\nHi.\n")
	if err != nil {
		t.Fatalf("expected draft file to be created: %v", err)
	}
	defer removeDraftFile(path)

	m := NewWithOptions(Options{backend: &draftFlowBackend{}})
	m.draftSerial = 3
	m.draft = draftState{Kind: composeDraft, Path: path, Serial: 3}

	next, _ := m.Update(draftEditorFinishedMsg{path: path, serial: 3, err: errors.New("exit status 1")})
	updated := next.(model)
	if updated.mode != draftReviewView {
		t.Fatalf("expected editor error to show draft review, got %v", updated.mode)
	}
	if updated.draft.Summary.Subject != "Saved" {
		t.Fatalf("expected saved draft summary, got %+v", updated.draft.Summary)
	}
	if !strings.Contains(updated.status, "editor reported an error") {
		t.Fatalf("expected editor warning status, got %q", updated.status)
	}
}

func TestPagedInboxShowsFirstPageThenLoadsOlderMail(t *testing.T) {
	backend := &pagedBackend{pages: [][]message{
		{
			{ID: "1", From: "Alice", Subject: "First"},
			{ID: "2", From: "Bob", Subject: "Second"},
		},
		{
			{ID: "2", From: "Bob", Subject: "Second"},
			{ID: "3", From: "Cora", Subject: "Older"},
		},
	}}
	m := NewWithOptions(Options{backend: backend})

	first := inboxPageFromCmd(t, m.Init())
	next, cmd := m.Update(first)
	updated := next.(model)
	if len(updated.messages) != 2 {
		t.Fatalf("expected first page to render immediately, got %+v", updated.messages)
	}
	if updated.loadingMore {
		t.Fatal("expected older mail to wait until the user reaches the bottom")
	}
	if updated.loadedAll {
		t.Fatal("expected first page not to mark the mailbox complete")
	}
	if updated.nextPage != 2 {
		t.Fatalf("expected next page to be 2, got %d", updated.nextPage)
	}
	if cmd != nil {
		t.Fatal("expected no automatic older-mail command")
	}

	next, cmd = updated.Update(keyMsg("j"))
	updated = next.(model)
	if cmd != nil {
		t.Fatal("expected first j to move to the bottom without loading")
	}
	next, cmd = updated.Update(keyMsg("j"))
	updated = next.(model)
	if !updated.loadingMore {
		t.Fatal("expected j at the bottom to load older mail")
	}
	if cmd == nil {
		t.Fatal("expected older page command")
	}

	second := cmd().(inboxPageLoadedMsg)
	next, cmd = updated.Update(second)
	updated = next.(model)
	if len(updated.messages) != 3 {
		t.Fatalf("expected older page to append without duplicate IDs, got %+v", updated.messages)
	}
	if updated.loadingMore {
		t.Fatal("expected loadingMore to stop after last page")
	}
	if !updated.loadedAll {
		t.Fatal("expected loadedAll after last page")
	}
	if cmd != nil {
		t.Fatal("expected no more page command")
	}
	if updated.nextPage != 0 {
		t.Fatalf("expected next page to be cleared after complete load, got %d", updated.nextPage)
	}
	if len(backend.calls) != 2 || backend.calls[0] != 1 || backend.calls[1] != 2 {
		t.Fatalf("expected page calls 1,2; got %v", backend.calls)
	}
}

func TestPagedInboxKeepsLoadingOlderMailFromBottom(t *testing.T) {
	backend := &pagedBackend{pages: [][]message{
		{
			{ID: "1", From: "Alice", Subject: "First"},
			{ID: "2", From: "Bob", Subject: "Second"},
		},
		{
			{ID: "3", From: "Cora", Subject: "Third"},
			{ID: "4", From: "Dax", Subject: "Fourth"},
		},
		{
			{ID: "5", From: "Eli", Subject: "Fifth"},
		},
	}}
	m := NewWithOptions(Options{backend: backend})

	first := inboxPageFromCmd(t, m.Init())
	next, _ := m.Update(first)
	updated := next.(model)
	updated.cursor = len(updated.messages) - 1

	next, cmd := updated.Update(keyMsg("j"))
	updated = next.(model)
	if !updated.loadingMore {
		t.Fatal("expected bottom j to start loading older mail")
	}

	second := inboxPageFromCmd(t, cmd)
	next, cmd = updated.Update(second)
	updated = next.(model)
	if len(updated.messages) != 4 || updated.messages[3].ID != "4" {
		t.Fatalf("expected second page to append and follow the bottom, got cursor=%d messages=%+v", updated.cursor, updated.messages)
	}
	if updated.cursor != 3 {
		t.Fatalf("expected cursor to follow appended older mail, got %d", updated.cursor)
	}
	if !updated.loadingMore {
		t.Fatal("expected app to keep loading older mail")
	}
	if cmd == nil {
		t.Fatal("expected automatic third page load")
	}

	third := inboxPageFromCmd(t, cmd)
	next, cmd = updated.Update(third)
	updated = next.(model)
	if len(updated.messages) != 5 || updated.messages[4].ID != "5" {
		t.Fatalf("expected all pages to be loaded, got %+v", updated.messages)
	}
	if updated.loadingMore {
		t.Fatal("expected loadingMore to stop after final page")
	}
	if !updated.loadedAll {
		t.Fatal("expected loadedAll after final page")
	}
	if cmd != nil {
		t.Fatal("expected no command after final page")
	}
	if len(backend.calls) != 3 || backend.calls[0] != 1 || backend.calls[1] != 2 || backend.calls[2] != 3 {
		t.Fatalf("expected calls 1,2,3; got %v", backend.calls)
	}
}

func TestPageRefreshDedupesAndPreservesSelection(t *testing.T) {
	m := NewWithOptions(Options{backend: &pagedBackend{}})
	m.messages = []message{
		{ID: "1", From: "Alice", Subject: "First"},
		{ID: "2", From: "Bob", Subject: "Second"},
	}
	m.cursor = 1
	m.loading = false
	m.loadedAll = false
	m.loadSerial = 7

	next, _ := m.Update(inboxPageLoadedMsg{
		page:   1,
		serial: 7,
		done:   false,
		messages: []message{
			{ID: "new", From: "News", Subject: "New message"},
			{ID: "new", From: "News", Subject: "New message duplicate"},
			{ID: "2", From: "Bob", Subject: "Second"},
			{ID: "1", From: "Alice", Subject: "First"},
		},
	})
	updated := next.(model)
	if len(updated.messages) != 3 {
		t.Fatalf("expected duplicate refresh IDs to be removed, got %+v", updated.messages)
	}
	if updated.cursor != 1 || updated.messages[updated.cursor].ID != "2" {
		t.Fatalf("expected cursor to stay on message 2, cursor=%d messages=%+v", updated.cursor, updated.messages)
	}
	if updated.nextPage != 2 {
		t.Fatalf("expected next page to remain page 2, got %d", updated.nextPage)
	}
}

func TestLastSessionRestoresReaderAfterInboxLoad(t *testing.T) {
	backend := &pagedBackend{pages: [][]message{{
		{ID: "1", From: "Alice", Subject: "First"},
		{ID: "2", From: "Bob", Subject: "Second", Body: "Saved body", BodyLoaded: true},
	}}}
	m := NewWithOptions(Options{
		backend: backend,
		LastSession: LastSession{
			Mailbox:      "INBOX",
			MessageID:    "2",
			ReaderOpen:   true,
			ReaderOffset: 3,
		},
	})

	first := inboxPageFromCmd(t, m.Init())
	next, cmd := m.Update(first)
	updated := next.(model)

	if cmd != nil {
		t.Fatal("expected cached restored reader to avoid extra command")
	}
	if updated.mode != readerView || updated.cursor != 1 || updated.readerOffset != 3 {
		t.Fatalf("expected restored reader on message 2 at offset 3, mode=%v cursor=%d offset=%d", updated.mode, updated.cursor, updated.readerOffset)
	}
}

func TestLastSessionLoadsOlderPagesUntilMessageIsFound(t *testing.T) {
	backend := &pagedBackend{pages: [][]message{
		{{ID: "1", From: "Alice", Subject: "First"}},
		{{ID: "9", From: "Nora", Subject: "Saved"}},
	}}
	m := NewWithOptions(Options{
		backend: backend,
		LastSession: LastSession{
			Mailbox:   "INBOX",
			MessageID: "9",
		},
	})

	first := inboxPageFromCmd(t, m.Init())
	next, cmd := m.Update(first)
	updated := next.(model)
	if cmd == nil || !updated.loadingMore {
		t.Fatal("expected restore to load older mail while looking for saved message")
	}

	second := inboxPageFromCmd(t, cmd)
	next, cmd = updated.Update(second)
	updated = next.(model)
	if cmd != nil {
		t.Fatal("expected restore to stop after finding saved message")
	}
	if updated.mode != inboxView || updated.cursor != 1 || updated.messages[updated.cursor].ID != "9" {
		t.Fatalf("expected cursor on saved older message, cursor=%d messages=%+v", updated.cursor, updated.messages)
	}
}

func TestWideInboxPreviewsSelectedMessage(t *testing.T) {
	backend := &bodyBackend{body: "Preview body"}
	m := NewWithOptions(Options{backend: backend})
	m.messages = []message{
		{ID: "1", From: "Alice", Subject: "First", Unread: true},
		{ID: "2", From: "Bob", Subject: "Second", Unread: true},
	}
	m.loading = false

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	updated := next.(model)
	if cmd == nil {
		t.Fatal("expected wide inbox to start preview loading")
	}
	if updated.loadingMessageID != "1" {
		t.Fatalf("expected first message preview to load, got %q", updated.loadingMessageID)
	}

	loaded := cmd().(messageBodyLoadedMsg)
	next, _ = updated.Update(loaded)
	updated = next.(model)
	if !updated.messages[0].BodyLoaded || updated.messages[0].Body != "Preview body" {
		t.Fatalf("expected first message body to be cached, got %+v", updated.messages[0])
	}
	if !updated.messages[0].Unread {
		t.Fatal("expected preview loading to leave unread state alone")
	}

	backend.body = "Second preview"
	next, cmd = updated.Update(keyMsg("j"))
	updated = next.(model)
	if updated.cursor != 1 {
		t.Fatalf("expected cursor to move to second message, got %d", updated.cursor)
	}
	if cmd == nil {
		t.Fatal("expected moving selection to load the next preview")
	}
	loaded = cmd().(messageBodyLoadedMsg)
	next, _ = updated.Update(loaded)
	updated = next.(model)
	if !updated.messages[1].BodyLoaded || updated.messages[1].Body != "Second preview" {
		t.Fatalf("expected second message body to be cached, got %+v", updated.messages[1])
	}
	if backend.reads != 2 {
		t.Fatalf("expected two preview reads, got %d", backend.reads)
	}
}

func TestMailboxRefreshTickReloadsNewestPage(t *testing.T) {
	backend := &pagedBackend{pages: [][]message{
		{
			{ID: "fresh", From: "News", Subject: "New arrival"},
		},
	}}
	m := NewWithOptions(Options{backend: backend})
	m.messages = []message{{ID: "old", From: "Old", Subject: "Already shown"}}
	m.loading = false
	m.loadedAll = false
	m.nextPage = 3

	next, cmd := m.Update(mailboxRefreshTickMsg{})
	updated := next.(model)
	if cmd == nil {
		t.Fatal("expected refresh tick to schedule work")
	}
	if updated.nextPage != 1 {
		t.Fatalf("expected refresh to reset next page to 1, got %d", updated.nextPage)
	}
	if updated.status != "checking for new mail..." {
		t.Fatalf("expected checking status, got %q", updated.status)
	}

	loaded := inboxPageFromBatchCmd(t, cmd, 1)
	next, _ = updated.Update(loaded)
	updated = next.(model)
	if len(updated.messages) != 1 || updated.messages[0].ID != "fresh" {
		t.Fatalf("expected newest page after refresh, got %+v", updated.messages)
	}
}

func TestThemeKeyOpensThemePicker(t *testing.T) {
	t.Setenv("CLIBOX_THEME", "")
	m := newTestModel()

	m = pressKey(t, m, "t")
	if !m.showThemes {
		t.Fatal("expected t to open the theme picker")
	}
	if m.themeCursor != m.theme {
		t.Fatalf("expected theme cursor to start on active theme, got cursor %d and theme %d", m.themeCursor, m.theme)
	}
}

func TestSetupRequiredErrorOpensAccountSetupView(t *testing.T) {
	m := NewWithOptions(Options{backend: &configurableBackend{}})

	next, _ := m.Update(inboxLoadedMsg{err: setupRequiredError{}})
	updated := next.(model)
	if updated.mode != setupView {
		t.Fatalf("expected setup view, got %v", updated.mode)
	}
	if updated.setupAccount != "personal" {
		t.Fatalf("expected default setup account personal, got %q", updated.setupAccount)
	}
	if updated.setupStep != setupEmailStep {
		t.Fatalf("expected setup to start with email step, got %v", updated.setupStep)
	}
}

func TestSetupRequiredErrorWithKnownEmailOpensSecretStep(t *testing.T) {
	m := NewWithOptions(Options{backend: &configurableBackend{}})
	m.setupEmail = "freddy@gmail.com"
	m.setupAccount = "gmail"
	m.setupProvider = detectProvider(m.setupEmail)

	next, _ := m.Update(inboxLoadedMsg{err: setupRequiredError{}})
	updated := next.(model)
	if updated.mode != setupView {
		t.Fatalf("expected setup view, got %v", updated.mode)
	}
	if updated.setupStep != setupSecretStep {
		t.Fatalf("expected setup to start at secret step, got %v", updated.setupStep)
	}
}

func TestSetupRequiredErrorWithNativeOAuthOpensReviewStep(t *testing.T) {
	m := NewWithOptions(Options{backend: &oauthConfigurableBackend{}})
	m.setupEmail = "freddy@gmail.com"
	m.setupAccount = "gmail"
	m.setupProvider = detectProvider(m.setupEmail)

	next, _ := m.Update(inboxLoadedMsg{err: setupRequiredError{}})
	updated := next.(model)
	if updated.mode != setupView {
		t.Fatalf("expected setup view, got %v", updated.mode)
	}
	if updated.setupStep != setupReviewStep {
		t.Fatalf("expected setup to start at review step, got %v", updated.setupStep)
	}
}

func TestSetupEmailDetectsProviderAndAccountName(t *testing.T) {
	m := NewWithOptions(Options{backend: &configurableBackend{}})
	m.mode = setupView
	m.setupEmail = ""

	m = pressKey(t, m, "freddy@gmail.com")
	if m.setupEmail != "freddy@gmail.com" {
		t.Fatalf("expected email input to append runes, got %q", m.setupEmail)
	}

	m = pressKey(t, m, "backspace")
	if m.setupEmail != "freddy@gmail.co" {
		t.Fatalf("expected backspace to drop last email rune, got %q", m.setupEmail)
	}

	m = pressKey(t, m, "m")
	m = pressKey(t, m, "enter")
	if m.setupStep != setupReviewStep {
		t.Fatalf("expected valid email to advance to review, got %v", m.setupStep)
	}
	if m.setupProvider.Name != "Gmail" {
		t.Fatalf("expected Gmail provider, got %q", m.setupProvider.Name)
	}
	if m.setupAccount != "gmail" {
		t.Fatalf("expected Gmail account name, got %q", m.setupAccount)
	}
}

func TestAccountSetupEnterMovesToSecretStep(t *testing.T) {
	m := NewWithOptions(Options{backend: &configurableBackend{}})
	m.mode = setupView
	m.setupStep = setupReviewStep
	m.setupEmail = "freddy@gmail.com"
	m.setupProvider = detectProvider(m.setupEmail)
	m.setupAccount = "personal"

	next, cmd := m.Update(keyMsg("enter"))
	updated := next.(model)
	if updated.setupStep != setupSecretStep {
		t.Fatalf("expected setup enter to move to secret step, got %v", updated.setupStep)
	}
	if cmd != nil {
		t.Fatal("expected review enter not to launch a wizard command")
	}
}

func TestSecretStepStartsBackgroundSetup(t *testing.T) {
	backend := &configurableBackend{}
	m := NewWithOptions(Options{backend: backend})
	m.mode = setupView
	m.setupStep = setupSecretStep
	m.setupEmail = "freddy@gmail.com"
	m.setupProvider = detectProvider(m.setupEmail)
	m.setupAccount = "personal"

	m = pressKey(t, m, "abcd efgh ijkl mnop")
	next, cmd := m.Update(keyMsg("enter"))
	updated := next.(model)
	if !updated.configuring {
		t.Fatal("expected secret enter to mark model as configuring")
	}
	if cmd == nil {
		t.Fatal("expected secret enter to return background setup command")
	}
	msg := cmd()
	configured, ok := msg.(accountConfiguredMsg)
	if !ok {
		t.Fatalf("expected accountConfiguredMsg, got %T", msg)
	}
	if configured.err != nil {
		t.Fatalf("expected fake setup to succeed: %v", configured.err)
	}
	if backend.saved.Account != "personal" || backend.saved.Email != "freddy@gmail.com" {
		t.Fatalf("unexpected saved setup: %+v", backend.saved)
	}
	if backend.saved.Secret != "abcd efgh ijkl mnop" {
		t.Fatalf("expected secret to be captured, got %q", backend.saved.Secret)
	}
}

func TestSecretStepCanOpenProviderHelp(t *testing.T) {
	m := NewWithOptions(Options{backend: &configurableBackend{}})
	m.mode = setupView
	m.setupStep = setupSecretStep
	m.setupEmail = "freddy@gmail.com"
	m.setupProvider = detectProvider(m.setupEmail)
	m.setupAccount = "gmail"

	next, cmd := m.Update(keyMsg("ctrl+o"))
	updated := next.(model)
	if cmd == nil {
		t.Fatal("expected ctrl+o to return browser open command")
	}
	if !strings.Contains(updated.status, "opening Gmail setup") {
		t.Fatalf("expected opening status, got %q", updated.status)
	}
}

func TestAccountSetupCanEditAccountName(t *testing.T) {
	m := NewWithOptions(Options{backend: &configurableBackend{}})
	m.mode = setupView
	m.setupStep = setupReviewStep
	m.setupAccount = "gmail"

	m = pressKey(t, m, "n")
	if m.setupStep != setupAccountStep {
		t.Fatalf("expected n to switch to account edit step, got %v", m.setupStep)
	}

	m.setupAccount = ""
	m = pressKey(t, m, "work")
	m = pressKey(t, m, "enter")
	if m.setupStep != setupReviewStep {
		t.Fatalf("expected edited account to return to review, got %v", m.setupStep)
	}
	if m.setupAccount != "work" {
		t.Fatalf("expected edited account name work, got %q", m.setupAccount)
	}
}

func TestProviderReviewCanOpenBrowserHelp(t *testing.T) {
	m := NewWithOptions(Options{backend: &configurableBackend{}})
	m.mode = setupView
	m.setupStep = setupReviewStep
	m.setupEmail = "freddy@gmail.com"
	m.setupProvider = detectProvider(m.setupEmail)

	next, cmd := m.Update(keyMsg("o"))
	updated := next.(model)
	if cmd == nil {
		t.Fatal("expected o to return browser open command")
	}
	if !strings.Contains(updated.status, "opening Gmail setup") {
		t.Fatalf("expected opening status, got %q", updated.status)
	}
}

func TestSetupScreensShowProviderURL(t *testing.T) {
	m := NewWithOptions(Options{backend: &configurableBackend{}})
	m.mode = setupView
	m.setupEmail = "freddy@gmail.com"
	m.setupProvider = detectProvider(m.setupEmail)
	m.setupAccount = "gmail"

	review := m.renderSetupReview(100, 30)
	secret := m.renderSetupSecret(100, 30)
	for name, view := range map[string]string{"review": review, "secret": secret} {
		if !strings.Contains(view, "https://myaccount.google.com/apppasswords") {
			t.Fatalf("expected %s view to show clickable provider URL, got:\n%s", name, view)
		}
	}
	if strings.Contains(secret, "previous screen") {
		t.Fatalf("expected secret view to offer a direct link, got:\n%s", secret)
	}
}

func TestAccountConfiguredReloadsInboxWithAccount(t *testing.T) {
	m := NewWithOptions(Options{backend: &configurableBackend{}})
	m.mode = setupView
	m.configuring = true

	next, cmd := m.Update(accountConfiguredMsg{account: "work"})
	updated := next.(model)
	if updated.mode != inboxView {
		t.Fatalf("expected inbox view after setup, got %v", updated.mode)
	}
	if updated.account != "work" {
		t.Fatalf("expected account to be updated, got %q", updated.account)
	}
	if !updated.loading {
		t.Fatal("expected inbox to reload after setup")
	}
	if cmd == nil {
		t.Fatal("expected setup completion to return reload command")
	}
}

func TestThemeCanBeSelectedFromEnvironment(t *testing.T) {
	t.Setenv("CLIBOX_THEME", "lagoon")

	m := New()
	if got := m.activeTheme().name; got != "Lagoon" {
		t.Fatalf("expected CLIBOX_THEME to select Lagoon, got %q", got)
	}
}

func TestCopperThemeAliasSelectsEmber(t *testing.T) {
	t.Setenv("CLIBOX_THEME", "copper")

	m := New()
	if got := m.activeTheme().name; got != "Ember" {
		t.Fatalf("expected copper alias to select Ember, got %q", got)
	}
}

func TestThemeCanBeSelectedFromConstructor(t *testing.T) {
	m := NewWithTheme("ember")
	if got := m.activeTheme().name; got != "Ember" {
		t.Fatalf("expected constructor theme Ember, got %q", got)
	}
}

func TestBlankConstructorThemeFallsBackToEnvironment(t *testing.T) {
	t.Setenv("CLIBOX_THEME", "lagoon")

	m := NewWithTheme("")
	if got := m.activeTheme().name; got != "Lagoon" {
		t.Fatalf("expected blank constructor theme to use environment, got %q", got)
	}
}

func TestUnknownThemeShowsFallbackStatus(t *testing.T) {
	m := NewWithTheme("banana")
	if got := m.activeTheme().name; got != "Nocturne" {
		t.Fatalf("expected unknown theme to fall back to Nocturne, got %q", got)
	}
	if !strings.Contains(m.status, `unknown theme "banana"`) {
		t.Fatalf("expected unknown theme status, got %q", m.status)
	}
}

func TestThemePickerPreviewsAndAppliesTheme(t *testing.T) {
	m := NewWithTheme("nocturne")

	m = pressKey(t, m, "t")
	m = pressKey(t, m, "j")
	if got := m.activeTheme().name; got != "Ember" {
		t.Fatalf("expected j in theme picker to preview Ember, got %q", got)
	}
	if !m.showThemes {
		t.Fatal("expected theme picker to stay open while previewing")
	}

	m = pressKey(t, m, "enter")
	if m.showThemes {
		t.Fatal("expected enter to close theme picker")
	}
	if got := m.activeTheme().name; got != "Ember" {
		t.Fatalf("expected enter to apply Ember, got %q", got)
	}
	if want := "theme Ember applied"; m.status != want {
		t.Fatalf("expected applied status %q, got %q", want, m.status)
	}
}

func TestThemePickerCanCancelPreview(t *testing.T) {
	m := NewWithTheme("nocturne")

	m = pressKey(t, m, "t")
	m = pressKey(t, m, "j")
	m = pressKey(t, m, "esc")
	if m.showThemes {
		t.Fatal("expected esc to close theme picker")
	}
	if got := m.activeTheme().name; got != "Nocturne" {
		t.Fatalf("expected esc to restore Nocturne, got %q", got)
	}
}

func TestThemePickerNumberAppliesTheme(t *testing.T) {
	m := NewWithTheme("nocturne")

	m = pressKey(t, m, "t")
	m = pressKey(t, m, "3")
	if got := m.activeTheme().name; got != "Lagoon" {
		t.Fatalf("expected number shortcut to apply Lagoon, got %q", got)
	}
	if m.showThemes {
		t.Fatal("expected number shortcut to close theme picker")
	}
}

func TestViewShowsThemeOnNarrowTerminal(t *testing.T) {
	m := NewWithTheme("lagoon")
	m.width = 34
	m.height = 10

	view := m.View()
	if !strings.Contains(view, "Lagoon") {
		t.Fatalf("expected narrow view to show active theme, got %q", view)
	}
}

func TestViewShowsThemePickerInsideTUI(t *testing.T) {
	m := NewWithTheme("lagoon")
	m.width = 72
	m.height = 22
	m = pressKey(t, m, "t")

	view := m.View()
	for _, want := range []string{"Themes", "Nocturne", "Ember", "Lagoon", "Enter applies"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected theme picker view to contain %q, got %q", want, view)
		}
	}
}

func TestThemeHelpListsCommands(t *testing.T) {
	help := ThemeHelp()
	for _, want := range []string{"nocturne", "ember", "lagoon", "clibox --theme lagoon", "theme picker"} {
		if !strings.Contains(help, want) {
			t.Fatalf("expected theme help to contain %q, got %q", want, help)
		}
	}
}

func TestThemesHaveDistinctVisibleSurfaces(t *testing.T) {
	seen := map[string]string{}
	for _, theme := range appThemes {
		signature := strings.Join([]string{
			theme.palette.background,
			theme.palette.header,
			theme.palette.surface,
			theme.palette.surfaceAlt,
			theme.palette.selected,
			theme.palette.footer,
		}, "|")
		if previous, ok := seen[signature]; ok {
			t.Fatalf("expected %s theme to differ from %s", theme.name, previous)
		}
		seen[signature] = theme.name
	}
}

func TestProviderDetectionGivesFriendlyGuidance(t *testing.T) {
	cases := map[string][]string{
		"freddy@gmail.com":      {"Gmail", "app password"},
		"freddy@icloud.com":     {"iCloud Mail", "app-specific password"},
		"freddy@outlook.com":    {"Outlook", "app password"},
		"freddy@yahoo.com":      {"Yahoo Mail", "app password"},
		"freddy@fastmail.com":   {"Fastmail", "app password"},
		"freddy@protonmail.com": {"Proton Mail", "Proton Mail Bridge"},
		"freddy@example.com":    {"Custom mail", "IMAP and SMTP"},
	}

	for email, wants := range cases {
		provider := detectProvider(email)
		combined := provider.Name + " " + provider.AuthSummary + " " + provider.SecretLabel + " " + provider.ManualWarning + " " + strings.Join(provider.Instructions, " ")
		for _, want := range wants {
			if !strings.Contains(combined, want) {
				t.Fatalf("expected provider guidance for %s to contain %q, got %+v", email, want, provider)
			}
		}
		if strings.Contains(provider.Name, "Gmail") && provider.HelpURL == "" {
			t.Fatal("expected Gmail to include browser setup URL")
		}
	}
}

func TestProtonProviderCanAutoConfigure(t *testing.T) {
	provider := detectProvider("freddy@proton.me")
	if !provider.canAutoConfigure() {
		t.Fatalf("expected Proton to auto-configure via Bridge defaults, got %+v", provider)
	}
	if provider.IMAPHost != "127.0.0.1" || provider.IMAPPort != 1143 {
		t.Fatalf("expected Proton Bridge IMAP 127.0.0.1:1143, got %s:%d", provider.IMAPHost, provider.IMAPPort)
	}
	if provider.SMTPHost != "127.0.0.1" || provider.SMTPPort != 1025 {
		t.Fatalf("expected Proton Bridge SMTP 127.0.0.1:1025, got %s:%d", provider.SMTPHost, provider.SMTPPort)
	}
}

func TestOpenURLRejectsNonWebSchemes(t *testing.T) {
	for _, rawURL := range []string{"", "notaurl", "https://", "file:///tmp/secret", "javascript:alert(1)"} {
		if err := openURL(rawURL); err == nil {
			t.Fatalf("expected %q to be rejected", rawURL)
		}
	}
}

func TestTerminalSafeTextRemovesControlSequences(t *testing.T) {
	input := "Alice\x1b[2J\x1b]52;c;SGVsbG8=\a red\x1b[31m text\x1b[0m\u202e done\tok\nnext"
	got := terminalSafeText(input)
	if got != "Alice red text done ok\nnext" {
		t.Fatalf("unexpected sanitized text: %q", got)
	}
	for _, unsafe := range []string{"\x1b", "\a", "]52", "[2J", "[31m", "\u202e"} {
		if strings.Contains(got, unsafe) {
			t.Fatalf("expected %q to be removed from %q", unsafe, got)
		}
	}
	if got := terminalSafeLine("subject\nspoofed\x1b[2J"); got != "subject spoofed" {
		t.Fatalf("unexpected sanitized line: %q", got)
	}
}

func TestFitFramePadsEveryRenderedLine(t *testing.T) {
	got := fitFrame("abc\nx", 5, 4)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d in %q", len(lines), got)
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width != 5 {
			t.Fatalf("line %d width = %d, want 5 in %q", i, width, got)
		}
	}
}

func TestFitFrameTruncatesOverwideStyledLines(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render("abcdef")
	got := fitFrame(styled+"\nxy", 5, 2)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d in %q", len(lines), got)
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width != 5 {
			t.Fatalf("line %d width = %d, want 5 in %q", i, width, got)
		}
	}
}

func TestWideInboxViewFitsTerminalWidth(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 16
	m.messages = []message{{
		ID:         "1",
		From:       "Very Long Sender Name",
		Email:      "sender@example.com",
		Subject:    strings.Repeat("long subject ", 20),
		Date:       "2026-06-10 19:47+00:00",
		Body:       strings.Repeat("long body ", 40),
		BodyLoaded: true,
	}}

	for i, line := range strings.Split(m.View(), "\n") {
		if width := lipgloss.Width(line); width > m.width {
			t.Fatalf("view line %d width = %d, want <= %d in %q", i, width, m.width, line)
		}
	}
}

func TestWideInboxFramePadsEveryLineAfterStyling(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.height = 14
	m.messages = []message{
		{ID: "1", From: "ATTACK SHARK", Email: "support@example.com", Subject: "Summer sale", Date: "2026-06-10", Body: "old body", BodyLoaded: true},
		{ID: "2", From: "Supabase", Email: "noreply@supabase.com", Subject: "Supa Update June 2026", Date: "2026-06-10", Body: "Everything that happened in the last month at Supabase", BodyLoaded: true},
	}
	m.cursor = 1

	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Fatalf("expected %d rows, got %d", m.height, len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width != m.width {
			t.Fatalf("line %d width = %d, want %d in %q", i, width, m.width, line)
		}
	}
}

func TestLoadedPreviewDoesNotDuplicateSnippet(t *testing.T) {
	m := newTestModel()
	msg := message{
		ID:         "1",
		Preview:    "Upgrade your gear with our premier festival offers.",
		Body:       "Upgrade your gear with our premier festival offers.\nEverything that happened in the last month.",
		BodyLoaded: true,
	}

	text := m.messageBodyText(msg, true)
	if strings.Count(text, "Upgrade your gear") != 1 {
		t.Fatalf("expected loaded body to omit duplicate preview snippet, got %q", text)
	}
}

func TestPreviewHeadersDoNotWrapLongSubjects(t *testing.T) {
	m := newTestModel()
	m.messages = []message{{
		ID:         "1",
		From:       "LinkedIn",
		Email:      "jobs@example.com",
		Subject:    strings.Repeat("junior software engineer ", 8),
		Date:       "2026-06-10",
		Body:       "Body text",
		BodyLoaded: true,
	}}

	preview := m.renderPreview(32, 8)
	lines := strings.Split(preview, "\n")
	if len(lines) != 8 {
		t.Fatalf("expected preview to fit requested height, got %d lines in %q", len(lines), preview)
	}
	if !strings.Contains(lines[3], "Date:") {
		t.Fatalf("expected long subject to stay on one header row, got preview:\n%s", preview)
	}
}

func TestReaderRendersLoadedImages(t *testing.T) {
	m := newTestModel()
	m.mode = readerView
	m.width = 100
	m.height = 28
	m.messages = []message{{
		ID:         "1",
		From:       "Alice",
		Email:      "alice@example.com",
		Subject:    "Image",
		Date:       "Today",
		Body:       "Photo attached.",
		Images:     []messageImage{{Name: "pixel.png", ContentType: "image/png", Data: []byte("hello")}},
		BodyLoaded: true,
	}}

	reader := m.View()
	if !strings.Contains(reader, "Image 1: pixel.png") {
		t.Fatalf("expected image fallback label, got:\n%s", reader)
	}
	if !strings.Contains(reader, "]1337;File=inline=1;") {
		t.Fatalf("expected inline image sequence, got:\n%s", reader)
	}
}

func TestWidePreviewDoesNotRenderImages(t *testing.T) {
	m := newTestModel()
	m.messages = []message{{
		ID:         "1",
		From:       "Alice",
		Email:      "alice@example.com",
		Subject:    "Image",
		Date:       "Today",
		Body:       "Photo attached.",
		Images:     []messageImage{{Name: "pixel.png", ContentType: "image/png", Data: []byte("hello")}},
		BodyLoaded: true,
	}}

	preview := m.renderPreview(80, 16)
	if strings.Contains(preview, "]1337;File=inline=1;") || strings.Contains(preview, "Image 1:") {
		t.Fatalf("expected preview to stay text-only, got:\n%s", preview)
	}
}

func TestInboxAndReaderSanitizeMailText(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 30
	m.messages = []message{{
		ID:         "1",
		From:       "Mallory\x1b]52;c;SGVsbG8=\a",
		Email:      "mallory@example.com",
		Subject:    "Reset\nspoof\x1b[2Jscreen",
		Date:       "Now\x1b[31m",
		Preview:    "Preview\x1b[5n",
		Body:       "Hello\x1b]0;owned\a world",
		BodyLoaded: true,
	}}

	inbox := m.View()
	for _, unsafe := range []string{"]52", "[2J", "[31m", "[5n", "]0;owned"} {
		if strings.Contains(inbox, unsafe) {
			t.Fatalf("inbox view leaked terminal control payload %q in %q", unsafe, inbox)
		}
	}

	m.mode = readerView
	reader := m.View()
	for _, unsafe := range []string{"]52", "[2J", "[31m", "[5n", "]0;owned"} {
		if strings.Contains(reader, unsafe) {
			t.Fatalf("reader view leaked terminal control payload %q in %q", unsafe, reader)
		}
	}
	if strings.Contains(reader, "Reset\nspoof") {
		t.Fatalf("reader view preserved unsafe subject newline: %q", reader)
	}
}

func TestHeaderSanitizesAccountLabel(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 30
	m.account = "work\x1b]52;c;SGVsbG8=\a"

	view := m.View()
	for _, unsafe := range []string{"\x1b", "\a", "]52"} {
		if strings.Contains(view, unsafe) {
			t.Fatalf("header leaked account control payload %q in %q", unsafe, view)
		}
	}
	if !strings.Contains(view, "work") {
		t.Fatalf("expected sanitized account name to remain visible, got %q", view)
	}
}

func TestGmailSecretNormalizationRemovesSpaces(t *testing.T) {
	provider := detectProvider("freddy@gmail.com")
	got := provider.normalizeSecret(" abcd efgh ijkl mnop ")
	if got != "abcdefghijklmnop" {
		t.Fatalf("expected Gmail app password spaces to be removed, got %q", got)
	}
}

func TestValidEmailAddress(t *testing.T) {
	for _, email := range []string{"freddy@gmail.com", "work@example.co"} {
		if !validEmailAddress(email) {
			t.Fatalf("expected %q to be valid", email)
		}
	}
	for _, email := range []string{"", "freddy", "@example.com", "freddy@example", "fre ddy@example.com"} {
		if validEmailAddress(email) {
			t.Fatalf("expected %q to be invalid", email)
		}
	}
}

func pressKey(t *testing.T, m model, key string) model {
	t.Helper()

	next, _ := m.Update(keyMsg(key))
	updated, ok := next.(model)
	if !ok {
		t.Fatalf("expected model update to return app model, got %T", next)
	}
	return updated
}

func newTestModel() model {
	m := New()
	m.messages = testMessages()
	m.loading = false
	m.status = ""
	return m
}

func testMessages() []message {
	return []message{
		{
			ID:         "1",
			From:       "Alice",
			Email:      "alice@example.com",
			Subject:    "Re: Design notes",
			Date:       "10:34 AM",
			Preview:    "I looked at the prototype.",
			Body:       "Hey Freddy,\n\nI looked at the prototype and left notes.",
			BodyLoaded: true,
			Unread:     true,
		},
		{
			ID:         "2",
			From:       "GitHub",
			Email:      "notifications@github.com",
			Subject:    "New issue assigned",
			Date:       "Yesterday",
			Preview:    "You were assigned issue #42.",
			Body:       "You were assigned issue #42 in freddyrosa16/clibox.",
			BodyLoaded: true,
			Unread:     true,
		},
	}
}

type bodyBackend struct {
	body  string
	reads int
}

func (b *bodyBackend) ListEnvelopes(context.Context) ([]message, error) {
	return testMessages(), nil
}

func (b *bodyBackend) ReadMessage(context.Context, message) (string, error) {
	b.reads++
	return b.body, nil
}

func (b *bodyBackend) Label() string {
	return "body fake"
}

type flagBackend struct {
	bodyBackend
	markedRead   []string
	markedUnread []string
	flaggedIDs   []string
	unflaggedIDs []string
}

func (b *flagBackend) ReadMessage(_ context.Context, msg message) (string, error) {
	b.reads++
	return b.body, nil
}

func (b *flagBackend) MarkMessageRead(_ context.Context, msg message) error {
	b.markedRead = append(b.markedRead, msg.ID)
	return nil
}

func (b *flagBackend) MarkMessageUnread(_ context.Context, msg message) error {
	b.markedUnread = append(b.markedUnread, msg.ID)
	return nil
}

func (b *flagBackend) SetMessageFlagged(_ context.Context, msg message, flagged bool) error {
	if flagged {
		b.flaggedIDs = append(b.flaggedIDs, msg.ID)
	} else {
		b.unflaggedIDs = append(b.unflaggedIDs, msg.ID)
	}
	return nil
}

type configurableBackend struct {
	account string
	saved   accountSetup
}

func (b configurableBackend) ListEnvelopes(context.Context) ([]message, error) {
	return testMessages(), nil
}

func (b configurableBackend) Label() string {
	return "fake " + b.account
}

func (b *configurableBackend) SaveAccountSetup(setup accountSetup) error {
	b.saved = setup
	return nil
}

func (b *configurableBackend) WithAccount(account string) inboxBackend {
	b.account = account
	return b
}

type oauthConfigurableBackend struct {
	configurableBackend
	oauthSaved accountSetup
}

func (b *oauthConfigurableBackend) SaveOAuthAccountSetup(_ context.Context, setup accountSetup) error {
	b.oauthSaved = setup
	return nil
}

type pagedBackend struct {
	pages [][]message
	calls []int
}

func (b *pagedBackend) ListEnvelopes(context.Context) ([]message, error) {
	var messages []message
	for _, page := range b.pages {
		messages = append(messages, page...)
	}
	return messages, nil
}

func (b *pagedBackend) ListEnvelopePage(_ context.Context, page int) ([]message, bool, error) {
	b.calls = append(b.calls, page)
	index := page - 1
	if index < 0 || index >= len(b.pages) {
		return nil, true, nil
	}
	return b.pages[index], index == len(b.pages)-1, nil
}

func (b *pagedBackend) Label() string {
	return "paged fake"
}

type draftFlowBackend struct {
	draft   string
	sent    string
	request draftRequest
}

func (b *draftFlowBackend) ListEnvelopes(context.Context) ([]message, error) {
	return testMessages(), nil
}

func (b *draftFlowBackend) Label() string {
	return "draft fake"
}

func (b *draftFlowBackend) PrepareDraft(_ context.Context, request draftRequest) (string, error) {
	b.request = request
	return b.draft, nil
}

func (b *draftFlowBackend) SendDraft(_ context.Context, content string) error {
	b.sent = content
	return nil
}

type actionFlowBackend struct {
	archived       string
	deleted        string
	searchQuery    string
	searchMessages []message
}

func (b *actionFlowBackend) ListEnvelopes(context.Context) ([]message, error) {
	return testMessages(), nil
}

func (b *actionFlowBackend) Label() string {
	return "action fake"
}

func (b *actionFlowBackend) SearchEnvelopePage(_ context.Context, _ int, query string) ([]message, bool, error) {
	b.searchQuery = query
	if b.searchMessages != nil {
		return b.searchMessages, true, nil
	}
	return testMessages(), true, nil
}

func (b *actionFlowBackend) ArchiveMessage(_ context.Context, msg message) error {
	b.archived = msg.ID
	return nil
}

func (b *actionFlowBackend) DeleteMessage(_ context.Context, msg message) error {
	b.deleted = msg.ID
	return nil
}

type mailboxSwitchTestBackend struct {
	mailbox string
	pages   map[string][][]message
	calls   []string
}

func (b *mailboxSwitchTestBackend) ListEnvelopes(context.Context) ([]message, error) {
	var messages []message
	for _, page := range b.pages[b.mailbox] {
		messages = append(messages, page...)
	}
	return messages, nil
}

func (b *mailboxSwitchTestBackend) ListEnvelopePage(_ context.Context, page int) ([]message, bool, error) {
	b.calls = append(b.calls, b.mailbox)
	pages := b.pages[b.mailbox]
	index := page - 1
	if index < 0 || index >= len(pages) {
		return nil, true, nil
	}
	return pages[index], index == len(pages)-1, nil
}

func (b *mailboxSwitchTestBackend) WithMailbox(mailbox string) inboxBackend {
	b.mailbox = mailbox
	return b
}

func (b *mailboxSwitchTestBackend) Label() string {
	return "mailbox switch fake"
}

func inboxPageFromCmd(t *testing.T, cmd tea.Cmd) inboxPageLoadedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected inbox page command")
	}
	msg := cmd()
	switch msg := msg.(type) {
	case inboxPageLoadedMsg:
		return msg
	case tea.BatchMsg:
		return inboxPageFromBatchMsg(t, msg, 0)
	default:
		t.Fatalf("expected inboxPageLoadedMsg, got %T", msg)
		return inboxPageLoadedMsg{}
	}
}

func inboxPageFromBatchCmd(t *testing.T, cmd tea.Cmd, index int) inboxPageLoadedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected batch command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	return inboxPageFromBatchMsg(t, batch, index)
}

func inboxPageFromBatchMsg(t *testing.T, batch tea.BatchMsg, index int) inboxPageLoadedMsg {
	t.Helper()
	if index < 0 || index >= len(batch) {
		t.Fatalf("batch index %d out of range for %d commands", index, len(batch))
	}
	if batch[index] == nil {
		t.Fatalf("batch command %d is nil", index)
	}
	msg := batch[index]()
	loaded, ok := msg.(inboxPageLoadedMsg)
	if !ok {
		t.Fatalf("expected inboxPageLoadedMsg, got %T", msg)
	}
	return loaded
}

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "delete":
		return tea.KeyMsg{Type: tea.KeyDelete}
	case "ctrl+o":
		return tea.KeyMsg{Type: tea.KeyCtrlO}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "ctrl+w":
		return tea.KeyMsg{Type: tea.KeyCtrlW}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}
