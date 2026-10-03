package srv

// Chat safety / protection of minors.
//
// There is no human moderation team, so everything here is automated and
// conservative: a normalising content filter (profanity, hate, sexual
// content, grooming patterns, personal-data / contact exchange), per-player
// rate limits, a strike ledger with escalating mutes, user reports that act
// immediately (hide + block for the reporter, escalating mutes on multiple
// independent reports), per-player blocks, per-session chat modes
// (free / quick-phrases / off) and a notice-and-action e-mail trail
// (DSA Art. 16) with signed one-click ban links so the operator can act
// from the mailbox without an admin UI.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"srv.exe.dev/db/dbgen"
)

// ---- configuration ----

// safetyEmail is the notice-and-action mailbox. Stored char-shifted (+3),
// like the address in the legal pages, so the plain string never appears in
// the binary or the repository.
var safetyEmailShifted = []int{117, 100, 105, 105, 100, 104, 111, 107, 108, 102, 110, 108, 118, 102, 107, 46, 110, 114, 107, 111, 118, 102, 107, 122, 100, 117, 125, 67, 106, 112, 100, 108, 111, 49, 102, 114, 112}

func safetyEmail() string {
	if e := os.Getenv("SAFETY_EMAIL"); e != "" {
		return e
	}
	var b strings.Builder
	for _, c := range safetyEmailShifted {
		b.WriteRune(rune(c - 3))
	}
	return b.String()
}

// safetyFallbackEmail: the exe.dev gateway only delivers to known
// addresses; if the primary alias is not (yet) whitelisted we fall back to
// the VM owner so no report is ever lost.
func safetyFallbackEmail() string { return os.Getenv("SAFETY_EMAIL_FALLBACK") }

const (
	chatRateWindow    = 10 * time.Second
	chatRateMax       = 5
	chatMaxLen        = 300
	strikesMute10m    = 3
	strikesMute24h    = 6
	strikesBan        = 10
	emailPerTargetTTL = time.Hour
)

var quickPhrases = []string{
	"Hallo! 👋", "Gut gespielt! 👏", "Danke!", "Ja", "Nein", "Schau mal hier! 📍",
	"Ich brauche Hilfe", "Wollen wir tauschen?", "Bis später!", "Glückwunsch! 🎉",
	"Schöne Parzelle!", "Lass uns Natur schützen 🌿", "Gute Idee!", "Moment...",
}

var validChatModes = map[string]bool{"free": true, "quick": true, "off": true}

// ---- normalisation ----

var leet = map[rune]rune{'0': 'o', '1': 'i', '3': 'e', '4': 'a', '5': 's', '7': 't', '@': 'a', '$': 's', '€': 'e', '!': 'i', '|': 'l', '+': 't'}

// normalizeText lowercases, strips diacritics, folds leetspeak and collapses
// repeated letters. Returns the spaced form ("f u c k" → "f u c k") and a
// squashed form with all non-letters removed ("fuck").
func normalizeText(s string) (spaced, squashed string) {
	s = strings.ToLower(s)
	s = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss").Replace(s)
	s = norm.NFD.String(s)
	// Leet-fold only tokens that mix letters with digits/symbols, so plain
	// numbers ("300 Münzen", "ich bin 12") keep their digits.
	toks := strings.Fields(s)
	for i, t := range toks {
		hasL, hasO := false, false
		for _, r := range t {
			if unicode.IsLetter(r) {
				hasL = true
			} else if _, ok := leet[r]; ok {
				hasO = true
			}
		}
		if hasL && hasO {
			toks[i] = strings.Map(func(r rune) rune {
				if l, ok := leet[r]; ok {
					return l
				}
				return r
			}, t)
		}
	}
	s = strings.Join(toks, " ")
	var sp, sq strings.Builder
	var last rune
	rep := 0
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == last {
			rep++
			if rep >= 2 {
				continue
			}
		} else {
			rep = 0
		}
		last = r
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sp.WriteRune(r)
			if unicode.IsLetter(r) {
				sq.WriteRune(r)
			}
		} else {
			sp.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(sp.String()), " "), sq.String()
}

// ---- rules ----

type filterResult struct {
	Severity int    // 0 ok, 1 masked (mild), 2 blocked + strike, 3 blocked + heavy strike + alert
	Category string // profanity, hate, sexual, grooming, contact, self_disclosure, spam
	Message  string // shown to the sender (German)
	Masked   string // message with mild words replaced by ***
}

var (
	// mild profanity → masked, no strike
	mildWords = wordSet("scheisse scheiss kacke arsch arschloch idiot depp trottel vollidiot wichser pisser penner spinner shit fuck fucking fuk bitch asshole bastard crap dumbass")
	// serious insults / hate → blocked, strike
	hateWords = wordSet("hurensohn hurnsohn fotze schlampe nutte hure missgeburt spast spasti mongo neger nigger nigga kanake kanacke kanack tschusch zigeuner schwuchtel transe judensau heilhitler sieghheil sieg_heil kys whore slut cunt faggot fag retard tranny kill_yourself bring_dich_um toete_dich geh_sterben")
	// sexual content → blocked; 3 in combination with age/child terms
	sexualWords = wordSet("sex sexy nackt nacktbild nacktbilder nudes nude dickpic penis schwanz vagina muschi blowjob blasen porno porn titten boobs wichsen masturbier horny sexting ficken fick gefickt bumsen onlyfans")
	minorWords  = wordSet("kind kinder maedchen maedel bub junge kleine kleiner jahre jaehrig teen teenager schueler schuelerin volksschule gymnasium schule eltern mama papa")
)

// wordSet builds a word-boundary alternation. Entries are separated by
// whitespace; multi-word phrases use "_" for the space.
func wordSet(s string) *regexp.Regexp {
	parts := strings.Fields(s)
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(regexp.QuoteMeta(p), "_", " ")
	}
	return regexp.MustCompile(`\b(` + strings.Join(parts, "|") + `)\b`)
}

// squashed-form substrings for slurs people spell out with separators (n.i.g.g.e.r, h u r e n s o h n)
var squashSlurs = []string{"hurensohn", "nigger", "nigga", "fotze", "faggot", "kanake", "schwuchtel", "heilhitler", "siegheil", "missgeburt"}

var groomingPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bwie alt bist du\b`), regexp.MustCompile(`\bhow old (are|r) (you|u)\b`),
	regexp.MustCompile(`\bbist du (gerade )?(allein|alleine|alleine zuhause|allein zuhause)\b`), regexp.MustCompile(`\bare (you|u) (home )?alone\b`),
	regexp.MustCompile(`\bsag(s| es| das)? (bitte )?(niemand|niemandem|keinem|deinen eltern nicht|nicht deinen eltern)\b`),
	regexp.MustCompile(`\bdon?t tell (anyone|anybody|your (parents|mom|dad|mum))\b`),
	regexp.MustCompile(`\b(unser|bleibt unser) (kleines )?geheimnis\b`), regexp.MustCompile(`\bour (little )?secret\b`),
	regexp.MustCompile(`\bschick(e|st)? (mir )?(mal )?(ein |n |a )?(foto|bild|pic|selfie|video)\b`),
	regexp.MustCompile(`\bsend (me )?(a )?(pic|pics|photo|selfie|video|nude)\b`),
	regexp.MustCompile(`\bwo wohnst du\b`), regexp.MustCompile(`\bwhere do (you|u) live\b`),
	regexp.MustCompile(`\b(welche|in welche) schule\b`), regexp.MustCompile(`\bwhat school\b`),
	regexp.MustCompile(`\b(treffen wir uns|wollen wir uns treffen|lass uns treffen|kannst du (rauskommen|raus kommen))\b`),
	regexp.MustCompile(`\b(let'?s meet|lets meet|meet up|meet me|wanna meet)\b`),
	regexp.MustCompile(`\bhast du (eine )?(freundin|freund|webcam|cam)\b`),
	regexp.MustCompile(`\b(zieh dich aus|mach die cam an|kamera an)\b`),
	regexp.MustCompile(`\bich (schenke|schenk|geb|gebe|zahle|zahl) dir (geld|muenzen|robux|vbucks|coins)\b`),
}

var selfDisclosurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bich bin (erst )?(\d{1,2}|zehn|elf|zwoelf|dreizehn|vierzehn|fuenfzehn|sechzehn|siebzehn)( jahre alt| jahre| jahr| j|$)`),
	regexp.MustCompile(`\bi(m| am) (\d{1,2}|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen)( years old| years| yo| y o|$)`),
	regexp.MustCompile(`\bich (wohne|wohn|lebe) in\b`), regexp.MustCompile(`\bi live (in|at)\b`),
	regexp.MustCompile(`\bmeine (adresse|handynummer|nummer|telefonnummer) (ist|lautet)\b`),
	regexp.MustCompile(`\bich (gehe|geh) (in die|auf die|ins) [a-z ]{0,20}(schule|gymnasium|volksschule|nms|hak|htl|klasse)\b`),
	regexp.MustCompile(`\bich (heisse|heiss) [a-z]+ [a-z]+\b`), // full real name
}

var (
	rxPhone     = regexp.MustCompile(`(?:\+|00)?\d[\d \-/().]{7,}\d`)
	rxEmail     = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+\s*(?:@|\(at\)|\[at\]| at )\s*[a-z0-9.\-]+\s*(?:\.|\(dot\)|\[dot\]| dot )\s*[a-z]{2,}`)
	rxURL       = regexp.MustCompile(`(?i)(?:https?://|www\.|\b[a-z0-9\-]+\.(?:com|at|de|net|org|io|gg|me|tv|xyz|ly|to|cc|link|app|chat)\b)`)
	rxHandle    = regexp.MustCompile(`(?:^|\s)@[a-z0-9_.]{3,}`)
	rxSocial    = regexp.MustCompile(`\b(snap|snapchat|snapp|insta|instagram|ig|tiktok|tik tok|whatsapp|wa|telegram|tg|discord|dc|signal|kik|omegle|threema|skype|zoom|facetime|ft|nummer|handynummer|telefonnummer|number|phone)\b`)
	rxSocialCtx = regexp.MustCompile(`\b(add|adde|addet|adden|folg|folge|follow|schreib|schreibe|write|dm|text|mein|meine|my|ist|is|hast du|have you|got|gib|gib mir|give|send|schick|hol dir|geh auf|komm auf|join)\b`)
)

func digitsIn(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

// filterChat classifies a message.
func filterChat(raw string) filterResult {
	spaced, squashed := normalizeText(raw)
	lowerRaw := strings.ToLower(raw)

	// -- personal data / contact exchange (the #1 grooming vector) --
	if m := rxPhone.FindString(raw); m != "" && digitsIn(m) >= 8 {
		return filterResult{2, "contact", "🔒 Bitte keine Telefonnummern im Chat teilen – zu deiner Sicherheit.", ""}
	}
	if rxEmail.MatchString(raw) {
		return filterResult{2, "contact", "🔒 Bitte keine E-Mail-Adressen im Chat teilen.", ""}
	}
	if rxURL.MatchString(lowerRaw) {
		return filterResult{2, "contact", "🔒 Links sind im Chat nicht erlaubt.", ""}
	}
	if rxHandle.MatchString(lowerRaw) || (rxSocial.MatchString(spaced) && rxSocialCtx.MatchString(spaced)) {
		return filterResult{2, "contact", "🔒 Bitte keine Kontaktdaten oder Social-Media-Namen austauschen – der Chat bleibt hier im Spiel.", ""}
	}

	// -- grooming / sexual --
	sexual := sexualWords.MatchString(spaced)
	minor := minorWords.MatchString(spaced)
	for _, rx := range groomingPatterns {
		if rx.MatchString(spaced) {
			sev := 2
			if sexual || minor {
				sev = 3
			}
			return filterResult{sev, "grooming", "⛔ Diese Nachricht wurde blockiert. Fragen nach Alter, Wohnort, Fotos oder Treffen sind hier nicht erlaubt.", ""}
		}
	}
	if sexual {
		sev := 2
		if minor {
			sev = 3
		}
		return filterResult{sev, "sexual", "⛔ Sexuelle Inhalte sind hier nicht erlaubt.", ""}
	}

	// -- hate / serious insults --
	if hateWords.MatchString(spaced) {
		return filterResult{2, "hate", "⛔ Beleidigungen und Hassrede sind nicht erlaubt.", ""}
	}
	for _, w := range squashSlurs {
		if strings.Contains(squashed, w) {
			return filterResult{2, "hate", "⛔ Beleidigungen und Hassrede sind nicht erlaubt.", ""}
		}
	}

	// -- self disclosure by (possibly) minors: block, friendly --
	for _, rx := range selfDisclosurePatterns {
		if rx.MatchString(spaced) {
			return filterResult{2, "self_disclosure", "🔒 Bitte verrate im Chat nichts Persönliches über dich (Alter, Wohnort, Schule, echter Name).", ""}
		}
	}

	// -- spam heuristics --
	if len(raw) > 40 && strings.ToUpper(raw) == raw && strings.ToLower(raw) != raw {
		return filterResult{1, "spam", "", strings.ToLower(raw)}
	}

	// -- mild profanity: mask --
	if mildWords.MatchString(spaced) {
		return filterResult{1, "profanity", "", maskMild(raw)}
	}
	return filterResult{0, "", "", raw}
}

// maskMild replaces mild words in the original text with asterisks. Works
// token-wise on the raw string so formatting is preserved.
func maskMild(raw string) string {
	toks := strings.Fields(raw)
	for i, t := range toks {
		sp, _ := normalizeText(t)
		if sp != "" && mildWords.MatchString(sp) {
			toks[i] = strings.Repeat("*", len([]rune(t)))
		}
	}
	return strings.Join(toks, " ")
}

// filterName checks a pseudonym at registration.
func filterName(name string) bool {
	r := filterChat(name)
	if r.Severity >= 2 {
		return false
	}
	spaced, squashed := normalizeText(name)
	if mildWords.MatchString(spaced) {
		return false
	}
	for _, w := range squashSlurs {
		if strings.Contains(squashed, w) {
			return false
		}
	}
	return true
}

// ---- rate limiting ----

type chatLimiter struct {
	mu   sync.Mutex
	hist map[string][]time.Time
	last map[string]string
	viol map[string]int // consecutive flood rejections
}

var chatLim = &chatLimiter{hist: map[string][]time.Time{}, last: map[string]string{}, viol: map[string]int{}}

// allow returns false when the player exceeded chatRateMax messages in
// chatRateWindow or repeats their previous message verbatim. strike is set
// on every 5th consecutive flood rejection (an eager player who hits the
// limit once is slowed down, not punished; a bot hammering the endpoint is).
func (l *chatLimiter) allow(pid, msg string) (ok bool, dup bool, strike bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	h := l.hist[pid]
	j := 0
	for _, t := range h {
		if now.Sub(t) < chatRateWindow {
			h[j] = t
			j++
		}
	}
	h = h[:j]
	if len(h) >= chatRateMax {
		l.hist[pid] = h
		l.viol[pid]++
		return false, false, l.viol[pid]%5 == 0
	}
	if l.last[pid] == msg {
		return false, true, false
	}
	l.viol[pid] = 0
	l.hist[pid] = append(h, now)
	l.last[pid] = msg
	if len(l.hist) > 5000 { // crude memory bound
		l.hist = map[string][]time.Time{}
		l.last = map[string]string{}
		l.viol = map[string]int{}
	}
	return true, false, false
}

// ---- strikes / mutes ----

func mutedUntil(p dbgen.Player) (time.Time, bool) {
	if p.ChatMutedUntil == nil {
		return time.Time{}, false
	}
	t := *p.ChatMutedUntil
	if t.After(time.Now()) {
		return t, true
	}
	return time.Time{}, false
}

func fmtRemaining(t time.Time) string {
	d := time.Until(t).Round(time.Minute)
	if d < time.Minute {
		return "unter einer Minute"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d Min.", int(d.Minutes()))
	}
	if d < 48*time.Hour {
		return fmt.Sprintf("%d Std.", int(d.Hours()))
	}
	return fmt.Sprintf("%d Tagen", int(d.Hours()/24))
}

// applyStrikes adds n strikes and escalates: 3 → 10 min mute, 6 → 24 h,
// 10 → permanent chat ban. Returns a human-readable consequence ("" if none).
func (s *Server) applyStrikes(ctx context.Context, pid string, n int64, reason string) string {
	total, err := s.Q.AddChatStrike(ctx, dbgen.AddChatStrikeParams{ChatStrikes: n, ID: pid})
	if err != nil {
		return ""
	}
	var until time.Time
	switch {
	case total >= strikesBan:
		s.Q.SetChatBan(ctx, pid)
		s.Q.LogSafetyEvent(ctx, dbgen.LogSafetyEventParams{Kind: "ban", PlayerID: &pid, Detail: reason})
		return "Dein Chat wurde dauerhaft gesperrt."
	case total >= strikesMute24h:
		until = time.Now().Add(24 * time.Hour)
	case total >= strikesMute10m:
		until = time.Now().Add(10 * time.Minute)
	default:
		return ""
	}
	s.Q.SetChatMute(ctx, dbgen.SetChatMuteParams{ChatMutedUntil: &until, ID: pid})
	s.Q.LogSafetyEvent(ctx, dbgen.LogSafetyEventParams{Kind: "mute", PlayerID: &pid, Detail: fmt.Sprintf("%s until %s", reason, until.Format(time.RFC3339))})
	return "Dein Chat ist für " + fmtRemaining(until) + " gesperrt."
}

func (s *Server) muteFor(ctx context.Context, pid string, d time.Duration, reason string) {
	p, err := s.Q.GetPlayerByID(ctx, pid)
	if err != nil {
		return
	}
	until := time.Now().Add(d)
	if cur, ok := mutedUntil(p); ok && cur.After(until) {
		return // already muted longer
	}
	s.Q.SetChatMute(ctx, dbgen.SetChatMuteParams{ChatMutedUntil: &until, ID: pid})
	s.Q.LogSafetyEvent(ctx, dbgen.LogSafetyEventParams{Kind: "mute", PlayerID: &pid, Detail: reason})
}

// ---- signed admin links ----

var (
	safetyKeyOnce sync.Once
	safetyKey     []byte
)

func (s *Server) hmacKey() []byte {
	safetyKeyOnce.Do(func() {
		if k := os.Getenv("SIEDLER_SECRET"); k != "" {
			safetyKey = []byte(k)
			return
		}
		path := filepath.Join(filepath.Dir(s.StaticDir), "..", "safety.key")
		if b, err := os.ReadFile(path); err == nil && len(b) >= 16 {
			safetyKey = bytes.TrimSpace(b)
			return
		}
		safetyKey = []byte(randomID(32))
		if err := os.WriteFile(path, safetyKey, 0o600); err != nil {
			slog.Warn("safety key not persisted; admin links expire on restart", "err", err)
		}
	})
	return safetyKey
}

func (s *Server) sign(parts ...string) string {
	m := hmac.New(sha256.New, s.hmacKey())
	m.Write([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

func (s *Server) adminLink(action, pid string) string {
	return fmt.Sprintf("https://%s/admin/safety/%s?p=%s&sig=%s", s.Hostname, action, pid, s.sign(action, pid))
}

// handleAdminSafety executes a signed one-click moderation action from the
// notice e-mail: ban / unban a player's chat, or unmute.
func (s *Server) handleAdminSafety(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	pid := r.URL.Query().Get("p")
	sig := r.URL.Query().Get("sig")
	if pid == "" || !hmac.Equal([]byte(sig), []byte(s.sign(action, pid))) {
		http.Error(w, "invalid link", 403)
		return
	}
	ctx := r.Context()
	p, err := s.Q.GetPlayerByID(ctx, pid)
	if err != nil {
		http.Error(w, "unknown player", 404)
		return
	}
	var msg string
	switch action {
	case "ban":
		s.Q.SetChatBan(ctx, pid)
		msg = "Chat dauerhaft gesperrt für " + p.Name
	case "unban":
		s.DB.ExecContext(ctx, "UPDATE players SET chat_banned = 0, chat_muted_until = NULL, chat_strikes = 0 WHERE id = ?", pid)
		msg = "Chat wieder freigegeben für " + p.Name
	case "mute7d":
		until := time.Now().Add(7 * 24 * time.Hour)
		s.Q.SetChatMute(ctx, dbgen.SetChatMuteParams{ChatMutedUntil: &until, ID: pid})
		msg = "Chat für 7 Tage gesperrt für " + p.Name
	default:
		http.Error(w, "unknown action", 404)
		return
	}
	s.Q.LogSafetyEvent(ctx, dbgen.LogSafetyEventParams{Kind: "admin_" + action, PlayerID: &pid, Detail: "via signed link"})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><body style="font-family:monospace;background:#1a1410;color:#f0d890;padding:2em"><h2>🛡️ %s</h2><p><a style="color:#9cf" href="%s">Rückgängig (unban)</a></p></body>`, msg, s.adminLink("unban", pid))
}

// ---- notice e-mail (DSA Art. 16 trail) ----

type mailThrottle struct {
	mu   sync.Mutex
	last map[string]time.Time
	hour []time.Time
}

var mailT = &mailThrottle{last: map[string]time.Time{}}

func (m *mailThrottle) allow(key string, perKey time.Duration, hourlyMax int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if t, ok := m.last[key]; ok && now.Sub(t) < perKey {
		return false
	}
	j := 0
	for _, t := range m.hour {
		if now.Sub(t) < time.Hour {
			m.hour[j] = t
			j++
		}
	}
	m.hour = m.hour[:j]
	if len(m.hour) >= hourlyMax {
		return false
	}
	m.hour = append(m.hour, now)
	m.last[key] = now
	return true
}

func sendMail(to, subject, body string) error {
	payload, _ := json.Marshal(map[string]string{"to": to, "subject": subject, "body": body})
	req, _ := http.NewRequest("POST", "http://169.254.169.254/gateway/email/send", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var res struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	if !res.Success {
		return fmt.Errorf("gateway: %s", res.Error)
	}
	return nil
}

// notifySafety e-mails the operator about an incident, throttled per target
// player. Runs in the background; failures only log.
func (s *Server) notifySafety(kind, sessionID string, target dbgen.Player, detail string) {
	if !mailT.allow(kind+":"+target.ID, emailPerTargetTTL, 20) {
		return
	}
	go func() {
		ctx := context.Background()
		var b strings.Builder
		fmt.Fprintf(&b, "Siedler Österreich – Sicherheitsmeldung (%s)\n\n", kind)
		fmt.Fprintf(&b, "Spieler: %s (id %s)\nStrikes: %d  gebannt: %d\nSession: %s\nZeit: %s\n\n%s\n\n",
			target.Name, target.ID, target.ChatStrikes, target.ChatBanned, sessionID, time.Now().Format(time.RFC3339), detail)
		if sessionID != "" {
			if msgs, err := s.Q.GetChatContext(ctx, sessionID); err == nil {
				b.WriteString("Letzte Nachrichten der Session (neueste zuerst; [H]=ausgeblendet):\n")
				for _, m := range msgs {
					h := " "
					if m.Hidden != 0 {
						h = "H"
					}
					fmt.Fprintf(&b, "[%s] #%d %s <%s>: %s\n", h, m.ID, m.CreatedAt.Format("15:04"), m.PlayerName, m.Message)
				}
			}
		}
		fmt.Fprintf(&b, "\nAktionen (signierte Links, kein Login nötig):\n  7 Tage sperren: %s\n  Dauerhaft sperren: %s\n  Entsperren: %s\n",
			s.adminLink("mute7d", target.ID), s.adminLink("ban", target.ID), s.adminLink("unban", target.ID))
		b.WriteString("\nAutomatisch erzeugt (DSA Art. 16 Melde- und Abhilfeverfahren). Aufbewahrung 180 Tage.\n")
		subj := fmt.Sprintf("[Siedler 🛡️] %s: %s", kind, target.Name)
		to := safetyEmail()
		if err := sendMail(to, subj, b.String()); err != nil {
			slog.Warn("safety mail failed", "to", to, "err", err)
			if fb := safetyFallbackEmail(); fb != "" {
				if err2 := sendMail(fb, subj+" (fallback)", "Primäre Adresse nicht zustellbar: "+err.Error()+"\n\n"+b.String()); err2 != nil {
					slog.Warn("safety mail fallback failed", "err", err2)
				}
			}
		}
	}()
}

// ---- retention ----

func (s *Server) safetyJanitor() {
	for {
		ctx := context.Background()
		if n, _ := s.Q.PurgeOldChat(ctx); n > 0 {
			slog.Info("safety janitor: purged chat", "rows", n)
		}
		s.Q.PurgeOldReports(ctx)
		s.Q.PurgeOldSafetyEvents(ctx)
		time.Sleep(6 * time.Hour)
	}
}
