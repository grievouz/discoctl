package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
	"github.com/ayn2op/arikawa/v3/utils/httputil/httpdriver"
	"github.com/ayn2op/arikawa/v3/utils/json/option"
)

const mutationRef = "223456789012345678/323456789012345678"

func TestMessageMutationDryRuns(t *testing.T) {
	for _, test := range []struct {
		args    []string
		stdin   string
		content string
	}{
		{[]string{"edit", mutationRef, "--text", "changed", "--dry-run"}, "", "changed"},
		{[]string{"edit", mutationRef, "--text", "help", "--dry-run"}, "", "help"},
		{[]string{"edit", mutationRef, "--text", "--help", "--dry-run"}, "", "--help"},
		{[]string{"edit", "--channel", "223456789012345678", "--message", "323456789012345678", "--stdin", "--no-mentions", "--ignore-unread", "--pretty", "--dry-run"}, "replacement\n", "replacement"},
		{[]string{"edit", "--msg", "alias", "--dry-run", mutationRef}, "", "alias"},
		{[]string{"delete", "https://discord.com/channels/@me/" + mutationRef, "--dry-run"}, "", ""},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := RunContext(context.Background(), append([]string{"messages"}, test.args...), strings.NewReader(test.stdin), &stdout, &stderr)
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				SchemaVersion string              `json:"schema_version"`
				Data          messageMutationPlan `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			plan := envelope.Data
			if test.args[0] == "edit" {
				if plan.AllowedMentions == nil {
					t.Fatal("edit plan must include mention settings")
				}
				if strings.Contains(strings.Join(test.args, " "), "--no-mentions") && len(*plan.AllowedMentions) != 0 {
					t.Fatal("mentions not suppressed")
				}
			}
			if envelope.SchemaVersion != schemaVersion || !plan.DryRun || plan.Applied || plan.Message.MessageID != "323456789012345678" {
				t.Fatalf("unexpected plan: %s", stdout.String())
			}
			if plan.Action != test.args[0]+"_message" {
				t.Fatalf("action = %q", plan.Action)
			}
			if test.content != "" && (plan.Content == nil || *plan.Content != test.content) {
				t.Fatalf("content = %v", plan.Content)
			}
		})
	}
}

func TestMessageMutationValidation(t *testing.T) {
	for _, args := range [][]string{
		{"delete", mutationRef}, // confirmation is required before authentication
		{"edit", mutationRef, "--dry-run"},
		{"edit", mutationRef, "--text", "a", "--stdin", "--dry-run"},
		{"edit", mutationRef, "--text", strings.Repeat("😀", 1001), "--dry-run"},
		{"edit", mutationRef, "--stdin", "--dry-run"}, // empty input
		{"edit", mutationRef, "--text", "x", "--channel", "223456789012345678", "--dry-run"},
		{"delete", "--message", "323456789012345678", "--dry-run"},
		{"delete", mutationRef, mutationRef, "--dry-run"},
		{"delete", "not-a-reference", "--dry-run"},
		{"edit", "--channel", "bad", "--message", "bad", "--text", "x", "--dry-run"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := RunContext(context.Background(), append([]string{"messages"}, args...), strings.NewReader(""), &stdout, &stderr)
			view, exit := classifyError(err)
			if err == nil || exit != exitInvalidArguments || view.Code != errorCodeInvalidArguments || stdout.Len() != 0 {
				t.Fatalf("error = %v, exit = %d, output = %s", err, exit, stdout.String())
			}
		})
	}
}

type mutationTransport func(*http.Request) (*http.Response, error)

func (f mutationTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// All HTTP is intercepted; these tests never connect to Discord.
func mutationAPI(transport mutationTransport) *api.Client {
	driver := httpdriver.WrapClient(http.Client{Transport: transport})
	return api.NewCustomClient("test-only-not-a-credential", httputil.NewClientWithDriver(driver))
}

func TestApplyMessageMutationHTTP(t *testing.T) {
	ref, err := parseMessageRef(mutationRef, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"edit", "delete"} {
		t.Run(action, func(t *testing.T) {
			for _, noMentions := range []bool{false, true} {
				mentions, _ := makeAllowedMentions(messageWriteOptions{NoMentions: noMentions, NoPingReply: true})
				data := api.EditMessageData{Content: option.NewNullableString("replacement"), AllowedMentions: mentions}
				calls, guards := 0, 0
				client := mutationAPI(func(req *http.Request) (*http.Response, error) {
					calls++
					wantMethod := "PATCH"
					if action == "delete" {
						wantMethod = "DELETE"
					}
					if guards != 1 || req.Method != wantMethod || req.URL.Path != api.Path+"/channels/223456789012345678/messages/323456789012345678" {
						t.Fatalf("unexpected request %s %s (guards %d)", req.Method, req.URL.Path, guards)
					}
					status, body := http.StatusNoContent, ""
					if action == "edit" {
						var got map[string]any
						if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
							t.Fatal(err)
						}
						wantParse := []any{"users", "roles", "everyone"}
						if noMentions {
							wantParse = []any{}
						}
						want := map[string]any{"content": "replacement", "allowed_mentions": map[string]any{"parse": wantParse, "replied_user": false}}
						// No attachments/embeds/flags/components fields: retain existing message data.
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("body = %#v, want %#v", got, want)
						}
						status = http.StatusOK
						body = `{"id":"323456789012345678","channel_id":"223456789012345678","content":"replacement","author":{"id":"123456789012345678"}}`
					} else if req.Body != nil {
						b, _ := io.ReadAll(req.Body)
						if len(b) != 0 {
							t.Fatalf("delete body = %s", b)
						}
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
				})
				var stdout bytes.Buffer
				err := applyMessageMutation(client, ref, messageMutationPlan{Action: action + "_message", Message: ref.view()}, data, func() error { guards++; return nil }, &outputOptions{}, &stdout)
				if err != nil {
					t.Fatal(err)
				}
				if calls != 1 {
					t.Fatalf("calls = %d", calls)
				}
				var envelope struct {
					Data map[string]any `json:"data"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if action == "delete" && envelope.Data["applied"] != true {
					t.Fatalf("delete result = %s", stdout.String())
				}
				if action == "edit" && envelope.Data["content"] != "replacement" {
					t.Fatalf("edit result = %s", stdout.String())
				}
			}
		})
	}
}

func TestMessageMutationGuardAndErrors(t *testing.T) {
	ref, err := parseMessageRef(mutationRef, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"edit_message", "delete_message"} {
		t.Run(action, func(t *testing.T) {
			blocked := errors.New("blocked unread guard")
			var stdout bytes.Buffer
			// A nil API client makes any request after guard failure fail the test.
			err := applyMessageMutation(nil, ref, messageMutationPlan{Action: action}, api.EditMessageData{}, func() error { return blocked }, &outputOptions{}, &stdout)
			if !errors.Is(err, blocked) || stdout.Len() != 0 {
				t.Fatalf("error = %v, output = %s", err, stdout.String())
			}
			for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
				calls := 0
				client := mutationAPI(func(req *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"code":50013,"message":"Missing Permissions"}`))}, nil
				})
				err := applyMessageMutation(client, ref, messageMutationPlan{Action: action, UnreadGuard: "bypassed"}, api.EditMessageData{}, func() error { t.Fatal("bypassed guard called"); return nil }, &outputOptions{}, &stdout)
				view, exit := classifyError(err)
				wantCode, wantExit := errorCodePermissionDenied, exitPermissionDenied
				if status == http.StatusNotFound {
					wantCode, wantExit = errorCodeNotFound, exitNotFound
				}
				if err == nil || view.Code != wantCode || exit != wantExit || stdout.Len() != 0 || calls != 1 {
					t.Fatalf("status %d: error %v, view %#v, exit %d, output %s, calls %d", status, err, view, exit, stdout.String(), calls)
				}
			}
		})
	}
}

func TestMessageMutationHelp(t *testing.T) {
	for _, args := range [][]string{
		{"edit", "--help"}, {"delete", "help"}, {"edit", mutationRef, "--help"}, {"delete", mutationRef, "-h"},
	} {
		var stdout, stderr bytes.Buffer
		if err := RunContext(context.Background(), append([]string{"messages"}, args...), strings.NewReader(""), &stdout, &stderr); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), "Usage:") || !strings.Contains(stdout.String(), "unread guard") {
			t.Fatalf("help = %s", stdout.String())
		}
	}
}
