package io

import (
	"encoding/json"
	"fmt"
	stdio "io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	smtpmod "go-version-rewrite/internal/smtp"
)

// Exercise the real MIME parser, both delivery entry points, and the Telegram
// SDK's HTTP serialization. No credentials or external Telegram calls are used.
func TestMailNotificationAPIEndToEnd(t *testing.T) {
	for _, mode := range []string{"single", "multi"} {
		for _, rejectEntities := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fallback=%v", mode, rejectEntities), func(t *testing.T) {
				type request struct {
					form     url.Values
					filename string
					data     string
				}
				requests := make(chan request, 8)
				var messageAttempts atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case strings.HasSuffix(r.URL.Path, "/getMe"):
						fmt.Fprint(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
						return
					case strings.HasSuffix(r.URL.Path, "/sendMessage"):
						if err := r.ParseForm(); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						requests <- request{form: r.Form}
						if messageAttempts.Add(1) == 1 && rejectEntities {
							w.WriteHeader(http.StatusBadRequest)
							fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities"}`)
							return
						}
					case strings.HasSuffix(r.URL.Path, "/sendDocument"):
						file, header, err := r.FormFile("document")
						if err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						defer file.Close()
						defer r.MultipartForm.RemoveAll()
						data, err := stdio.ReadAll(file)
						if err != nil {
							t.Error(err)
						}
						requests <- request{filename: header.Filename, data: string(data)}
					default:
						t.Errorf("unexpected Telegram endpoint %s", r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
						return
					}
					fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":101,"type":"private"}}}`)
				}))
				defer server.Close()
				bot, err := tgbotapi.NewBotAPIWithClient("test", server.URL+"/bot%s/%s", server.Client())
				if err != nil {
					t.Fatal(err)
				}

				raw := "From: =?UTF-8?B?5Lit5paH?= <sender@example.com>\r\n" +
					"To: user@recipient.example\r\nSubject: [example/project_name] Build & Release (failed)\r\n" +
					"MIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=outer\r\n\r\n" +
					"--outer\r\nContent-Type: multipart/alternative; boundary=inner\r\n\r\n" +
					"--inner\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nPlain version\r\n" +
					"--inner\r\nContent-Type: Text/HTML; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
					"<h2>Build &amp; Release</h2><p>[example/project_name] failed. 中文 👩‍💻</p>" +
					"<a href=3D\"https://example.com/run(test)?a=3D1&amp;b=3D2\"><strong>View workflow run</strong></a>\r\n" +
					"--inner--\r\n--outer\r\nContent-Type: image/png\r\nContent-Disposition: inline; filename=logo.png\r\n" +
					"Content-Transfer-Encoding: base64\r\n\r\nUE5H\r\n" +
					"--outer\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=report.txt\r\n" +
					"Content-Transfer-Encoding: base64\r\n\r\ncmVwb3J0\r\n--outer--\r\n"
				mail, err := smtpmod.ParseMail(strings.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				mail.EnvelopeFrom = "sender@example.com"
				mail.EnvelopeRecipients = []string{"user@recipient.example"}
				ioModule, _ := newTestIO(t)
				ioModule.domainToUser.Store("recipient.example", int64(101))
				if mode == "single" {
					ioModule.SetBot(bot)
					ioModule.HandleMail(mail)
				} else {
					ioModule.HandleMailMulti(mail, []DeliveryEndpoint{{Name: "test", Sender: bot}}, nil, time.Unix(0, 0))
				}

				var messages []url.Values
				documents := map[string]string{}
				for len(requests) > 0 {
					r := <-requests
					if r.form != nil {
						messages = append(messages, r.form)
					} else {
						documents[r.filename] = r.data
					}
				}
				wantMessages := 1
				if rejectEntities {
					wantMessages++
				}
				if len(messages) != wantMessages {
					t.Fatalf("got %d message requests, want %d", len(messages), wantMessages)
				}
				var entities []tgbotapi.MessageEntity
				if err := json.Unmarshal([]byte(messages[0].Get("entities")), &entities); err != nil {
					t.Fatal(err)
				}
				message := tgbotapi.NewMessage(101, messages[0].Get("text"))
				message.Entities = entities
				message.ParseMode = messages[0].Get("parse_mode")
				assertNativeMailEntities(t, message)
				if !strings.Contains(message.Text, "中文 <sender@example.com>") || !strings.Contains(message.Text, "[example/project_name] failed. 中文 👩‍💻") {
					t.Fatalf("literal header/body were damaged: %q", message.Text)
				}
				if messages[0].Get("disable_web_page_preview") != "true" {
					t.Fatal("newsletter links would expand into distracting webpage previews")
				}
				linkFound := false
				for _, entity := range entities {
					if entity.Type == "text_link" && entity.URL == "https://example.com/run(test)?a=1&b=2" {
						linkFound = entityText(t, message, entity) == "View workflow run"
					}
				}
				if !linkFound {
					t.Fatal("clickable action was not serialized with its intact URL")
				}
				if rejectEntities {
					fallback := messages[1]
					if fallback.Get("text") != message.Text || fallback.Get("parse_mode") != "" || (fallback.Get("entities") != "" && fallback.Get("entities") != "null") {
						t.Fatalf("fallback changed literal content or retained formatting: %v", fallback)
					}
					if fallback.Get("reply_markup") == "" || fallback.Get("reply_markup") != messages[0].Get("reply_markup") {
						t.Fatal("fallback lost mail action buttons")
					}
				}
				if documents["report.txt"] != "report" || documents["content.html"] != mail.HTML {
					t.Fatalf("attachments or full HTML changed: %v", documents)
				}
				if _, sent := documents["logo.png"]; sent {
					t.Fatal("inline newsletter logo was sent as a standalone document")
				}
			})
		}
	}
}
