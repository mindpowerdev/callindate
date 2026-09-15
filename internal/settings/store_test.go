package settings

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mindpowerdev/callindate.git/internal/storage"
)

func TestSmoke(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, ok, err := store.ChatID(ctx); err != nil || ok {
		t.Fatalf("chat_id не должен быть задан изначально: ok=%v err=%v", ok, err)
	}
	if err := store.SetChatID(ctx, 12345); err != nil {
		t.Fatal(err)
	}
	id, ok, err := store.ChatID(ctx)
	if err != nil || !ok || id != 12345 {
		t.Fatalf("ChatID = %d, %v, %v", id, ok, err)
	}

	dailyTime, err := store.DailyReminderTime(ctx)
	if err != nil || dailyTime != "09:00" {
		t.Fatalf("DailyReminderTime по умолчанию = %q, want 09:00 (err=%v)", dailyTime, err)
	}
	if err := store.SetDailyReminderTime(ctx, "08:30"); err != nil {
		t.Fatal(err)
	}
	dailyTime, err = store.DailyReminderTime(ctx)
	if err != nil || dailyTime != "08:30" {
		t.Fatalf("DailyReminderTime после изменения = %q (err=%v)", dailyTime, err)
	}

	if _, ok, err := store.LastDailyDigestDate(ctx); err != nil || ok {
		t.Fatalf("last_daily_digest_date не должен быть задан изначально: ok=%v err=%v", ok, err)
	}
	if err := store.SetLastDailyDigestDate(ctx, "2026-09-15"); err != nil {
		t.Fatal(err)
	}
	date, ok, err := store.LastDailyDigestDate(ctx)
	if err != nil || !ok || date != "2026-09-15" {
		t.Fatalf("LastDailyDigestDate = %q, %v (err=%v)", date, ok, err)
	}
}
