# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

CalinDate is a Telegram bot (Go) for tracking one child's after-school activities/clubs ("кружки и секции"). Data (schedule, payments, plans) is fully shared and unauthenticated: anyone who messages the bot reads/writes the same rows — there's no per-chat data partitioning anywhere in the schema, by design (e.g. both parents can independently manage the same schedule from their own private chats with the bot). The only per-chat concept is the *subscriber list* (`internal/settings`), which controls who receives proactive reminders. Menu: 📅 Календарь (today/tomorrow, attendance marking), 💳 Оплаты (payment log + payment plans), 📊 Статистика (week/month/academic-year reports), 🔔 Напоминания (daily digest time). A background scheduler proactively broadcasts to every subscriber: a daily digest, a reminder 1 hour before each activity, payment-due reminders, and low-abonement-balance warnings. All user-facing strings are in Russian. See "Known simplifications" below for what's still not supported (multi-child, per-user permissions).

Module path: `github.com/mindpowerdev/callindate.git` (note: name differs from the repo directory `calindate`).

## Language rules

- Обсуждай проект и отвечай пользователю только на русском языке.
- Основной язык разработки — Go (Golang).

## Commands

```bash
go build ./...              # build
go run ./cmd/bot             # run the bot locally (requires BOT_TOKEN; DB_PATH/REMINDER_TZ optional)
go test ./...                # run tests (all internal/* packages except cmd/bot itself)
go vet ./...                 # static checks
gofmt -l .                   # check formatting (no formatter config beyond stdlib gofmt)
```

### Configuration

Env vars (see `.env.example`): `BOT_TOKEN` (required, `log.Fatal` if unset), `DB_PATH` (default `calindate.db`), `REMINDER_TZ` (default `Europe/Moscow` — parsed once at startup via `time.LoadLocation` and threaded through as `*time.Location` for all reminder-time math). No `.env` loader — export vars in the shell/process manager.

## Architecture

Long polling via [go-telegram-bot-api/telegram-bot-api/v5](https://github.com/go-telegram-bot-api/telegram-bot-api) (`GetUpdatesChan`), not webhooks. Storage is SQLite via the pure-Go `modernc.org/sqlite` driver (no cgo) — one shared `*sql.DB` (opened by `internal/storage.Open`, owned/closed by `main`) used by independent per-domain stores, each self-migrating its own tables on `NewStore(db)`.

**`cmd/bot/main.go`** — wires everything: opens the DB, loads `REMINDER_TZ`, constructs `schedule.Store` / `payment.Store` / `settings.Store`, builds the Telegram client, calls `bot.New(...).Run()`.

### Domain packages (no Telegram dependency, easy to unit test)

- **`internal/schedule`** — the recurring weekly timetable *and* per-date attendance tracking.
  - `model.go`: `Activity` (name, `time.Weekday`, `"15:04"` start time) + Russian weekday/time parsing (`ParseWeekday`, `ParseStartTime`, `WeekdayName`).
  - `store.go`: `Add` (returns the new row's id), `All`, `ForWeekday` (read-only weekday preview, used for "Завтра" — no attendance possible for the future).
  - `occurrence.go`: **occurrences are materialized lazily**, not precomputed — `EnsureOccurrencesForDate(ctx, date)` does `INSERT ... ON CONFLICT DO NOTHING` for every activity whose weekday matches `date.Weekday()`, then returns them joined with activity info. This is how "Сегодня" and the scheduler both get a concrete, markable row without ever generating future dates. `SetOccurrenceStatus` records `attended|missed|rescheduled|not_marked`. `StatsForRange` computes *planned* counts by iterating weekdays in a date range (no occurrence rows needed) and *actual* counts by grouping real occurrence rows — both keyed by **activity name**, not id (see next point).
- **`internal/payment`** — payment log (manual, date/amount/lesson-count facts — no real payment processing) plus payment *plans*.
  - `model.go`: `Payment`, `ParseAmount`/`FormatAmount`/`LessonsWord` (Russian pluralization).
  - `store.go`: `Add`, `Recent`, `SumByActivity(from, to)`.
  - `plan.go`: `Plan` — one of 5 `PlanType`s (`monthly`, `semester`, `abonement`, `per_visit`, `one_time`). **Keyed by activity name, not `activity_id`**: one кружок can appear as multiple rows in `activities` (e.g. Mon + Wed), but its payment plan is singular. **Important gotcha already hit once**: SQLite's `COLLATE NOCASE` only folds ASCII case, not Cyrillic, so "Английский" vs "английский" would NOT match under it — matching is instead done via `normalizeName` (Go `strings.ToLower`) against a separate `name_key` column, with `activity_name` kept as-is for display. Any new lookup-by-name code must go through `normalizeName`, not raw SQL comparison. `AdvanceDue` (push `next_due_date` by 1 month / `IntervalMonths`), `AddLessons`/`DecrementLesson` (abonement balance, floored at 0), `MarkDueReminderSent`/`SetLowBalanceReminded` (dedup flags — see scheduler below).
- **`internal/settings`** — a key-value store (`store.go`: `daily_reminder_time`, `last_daily_digest_date`) plus the **subscribers table** (`subscribers.go`: `AddSubscriber`/`RemoveSubscriber`/`Subscribers`, one row per chat that ran `/start` and hasn't `/stop`-ped). This is the *only* per-chat state in the whole app — everything else is global/shared.
- **`internal/stats`** — `BuildReport(ctx, scheduleStore, paymentStore, from, to)` joins `schedule.StatsForRange` + `payment.SumByActivity` into a `Report` (per-activity planned/attended/missed/rescheduled + spend, `ActivityStats.CostPerVisit()`). Pure function of two stores + a date range — no Telegram, easy to test with synthetic data.

### `internal/bot` (Telegram-facing, split by concern)

- `bot.go` — `Bot` struct (api + all 3 stores + `*time.Location`), command dispatch (`/start /help /add /pay /plan /remind_time /stop`), shared helpers (`today()`, `reply`/`send`). `/start` calls `settings.AddSubscriber` (idempotent — re-running `/start` doesn't duplicate); `/stop` calls `RemoveSubscriber`.
- `menu.go` — all inline keyboards + the single `handleCallback` router, dispatching on `callback.Data` prefix (`menu:*`, `cal:*`, `pay:*`, `stats:*`, `occ:attend|miss|resched:<occurrence-id>`).
- `calendar.go` — "Сегодня" (`EnsureOccurrencesForDate` + one summary message + one follow-up message *per unmarked occurrence* carrying the 3 attendance buttons) vs "Завтра" (plain `ForWeekday` preview, no buttons — can't mark attendance for the future). Attendance callbacks call `SetOccurrenceStatus`, then `consumeAbonementLesson` (decrements an `abonement` plan on `attended`/`missed`, **not** `rescheduled`; fires the low-balance warning immediately via `broadcast` to *all* subscribers, not just whoever tapped the button).
- `payments.go` — `/pay` (unchanged 2-3-arg syntax; if the trailing name matches an existing `Plan`, it also calls `AdvanceDue` or `AddLessons` — this is the *only* place `/pay` and plans connect), `/plan <type> <params...> <name>` (params before name, same convention as `/add`/`/pay`, so a multi-word name doesn't break parsing), "Оплаты" submenu text builders.
- `stats.go` — period math (`periodRange`, `academicYearStart` — academic year = Sep 1 onward) + report formatting.
- `scheduler.go` — `runScheduler` is a 1-minute `time.Ticker` goroutine started from `Bot.Run()`. Each tick fetches `settings.Subscribers` and, for anything due, builds the message **once** and sends it to every subscriber via `broadcast`, only marking the dedup flag (`last_daily_digest_date` / `hour_reminder_sent` / `due_reminder_sent_for` / `low_balance_reminded`) *after* the full broadcast — those flags are global, not per-chat, so marking-then-looping would silently skip every subscriber after the first. **All four reminder checks are threshold-based, not exact-time-match** (see `dueDailyDigest`, `hourReminderDue`, `dueSoon`, all pure functions unit-tested in `scheduler_test.go`), so a bot restart never silently loses a reminder. No-ops entirely while the subscriber list is empty (i.e. before anyone has run `/start`).

## Known simplifications (deliberate, revisit if requirements change)

- Single child — no `child_id` anywhere. Multiple *people* are supported (see above), but not multiple *kids*.
- No auth/permissions at all: anyone who finds the bot on Telegram and sends `/start` gets full read/write access to the shared schedule/payments and joins the reminder broadcast list. Fine for a private family bot; would need real access control before wider use.
- Abonement lessons are decremented on `attended` **and** `missed` (a scheduled slot "burns" either way), not on `rescheduled`. Easy to flip in `calendar.go`'s `consumeAbonementLesson` if the real-world convention differs.
- Occurrences are only ever materialized for "today" (via the daily scheduler tick or opening "Сегодня") — if the bot is offline all day, that day's attendance can never be retroactively recorded.
