package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type User struct {
	JID          string   `json:"jid"`
	Nome         string   `json:"nome"`
	Coins        int64    `json:"coins"`
	XP           int64    `json:"xp"`
	Level        int64    `json:"level"`
	Daily        int64    `json:"daily"`
	Wins         int64    `json:"wins"`
	Losses       int64    `json:"losses"`
	Achievements []string `json:"achievements"`
	Inventory    []string `json:"inventory"`
	Spouse       string   `json:"spouse"`
	MarriedAt    int64    `json:"married_at"`
	CreatedAt    int64    `json:"created_at"`
}

type WarnEntry struct {
	Count   int      `json:"count"`
	Reasons []string `json:"reasons"`
}

type AFKEntry struct {
	Reason  string `json:"reason"`
	SinceTS int64  `json:"since_ts"`
}

type ScheduledClose struct {
	GroupID       string
	HorarioFechar string
	MinutosAbrir  int
}

type DB struct {
	sql        *sql.DB
	mu         sync.RWMutex
	isPostgres bool
}

func Open(dbDir string) (*DB, error) {
	var conn *sql.DB
	var err error
	var isPG bool

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		conn, err = sql.Open("pgx", dbURL)
		if err != nil {
			return nil, err
		}
		conn.SetMaxOpenConns(10)
		isPG = true
	} else {
		dbPath := filepath.Join(dbDir, "nyxbot.db")
		dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dbPath)
		conn, err = sql.Open("sqlite", dsn)
		if err != nil {
			return nil, err
		}
		conn.SetMaxOpenConns(1)
	}

	d := &DB{sql: conn, isPostgres: isPG}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) rebind(q string) string {
	if !d.isPostgres || !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	b.Grow(len(q) + 16)
	n := 1
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			n++
		} else {
			b.WriteByte(q[i])
		}
	}
	return b.String()
}

func (d *DB) exec(q string, args ...any) (sql.Result, error) {
	return d.sql.Exec(d.rebind(q), args...)
}

func (d *DB) query(q string, args ...any) (*sql.Rows, error) {
	return d.sql.Query(d.rebind(q), args...)
}

func (d *DB) queryRow(q string, args ...any) *sql.Row {
	return d.sql.QueryRow(d.rebind(q), args...)
}

func (d *DB) Close() error {
	return d.sql.Close()
}

func (d *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			jid TEXT PRIMARY KEY,
			nome TEXT DEFAULT '',
			coins BIGINT DEFAULT 0,
			xp BIGINT DEFAULT 0,
			level BIGINT DEFAULT 1,
			daily BIGINT DEFAULT 0,
			wins BIGINT DEFAULT 0,
			losses BIGINT DEFAULT 0,
			achievements TEXT DEFAULT '[]',
			inventory TEXT DEFAULT '[]',
			spouse TEXT DEFAULT '',
			married_at BIGINT DEFAULT 0,
			created_at BIGINT DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS donos (
			jid TEXT PRIMARY KEY
		);`,
		`CREATE TABLE IF NOT EXISTS premium_users (
			jid TEXT PRIMARY KEY,
			expires_at BIGINT DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS group_features (
			group_id TEXT NOT NULL,
			feature TEXT NOT NULL,
			enabled INTEGER DEFAULT 0,
			PRIMARY KEY (group_id, feature)
		);`,
		`CREATE TABLE IF NOT EXISTS group_prefixes (
			group_id TEXT PRIMARY KEY,
			prefix TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS active_groups (
			group_id TEXT PRIMARY KEY,
			expires_at BIGINT DEFAULT 0,
			activated_by TEXT DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS boas_vindas (
			group_id TEXT PRIMARY KEY,
			ativo INTEGER DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS mutes (
			group_id TEXT NOT NULL,
			user_jid TEXT NOT NULL,
			until_ts BIGINT DEFAULT 0,
			PRIMARY KEY (group_id, user_jid)
		);`,
		`CREATE TABLE IF NOT EXISTS warns (
			group_id TEXT NOT NULL,
			user_jid TEXT NOT NULL,
			count INTEGER DEFAULT 0,
			reasons TEXT DEFAULT '[]',
			PRIMARY KEY (group_id, user_jid)
		);`,
		`CREATE TABLE IF NOT EXISTS afk (
			user_jid TEXT PRIMARY KEY,
			reason TEXT DEFAULT '',
			since_ts BIGINT DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS blacklist (
			group_id TEXT NOT NULL,
			user_jid TEXT NOT NULL,
			reason TEXT DEFAULT '',
			added_by TEXT DEFAULT '',
			added_at BIGINT DEFAULT 0,
			PRIMARY KEY (group_id, user_jid)
		);`,
		`CREATE TABLE IF NOT EXISTS autoreply (
			group_id TEXT NOT NULL,
			trigger_word TEXT NOT NULL,
			response TEXT NOT NULL,
			PRIMARY KEY (group_id, trigger_word)
		);`,
		`CREATE TABLE IF NOT EXISTS agendamentos_fechar (
			group_id TEXT PRIMARY KEY,
			horario_fechar TEXT NOT NULL,
			minutos_abrir INTEGER DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS marriage_proposals (
			to_jid TEXT PRIMARY KEY,
			from_jid TEXT NOT NULL,
			created_at BIGINT DEFAULT 0
		);`,
	}

	for _, s := range stmts {
		if _, err := d.sql.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func normJID(jid string) string {
	jid = strings.TrimSpace(jid)
	if idx := strings.Index(jid, ":"); idx != -1 {
		if at := strings.Index(jid, "@"); at != -1 {
			jid = jid[:idx] + jid[at:]
		}
	}
	return jid
}

func (d *DB) GetUser(jid string) *User {
	jid = normJID(jid)
	d.mu.Lock()
	defer d.mu.Unlock()

	var u User
	var achJSON, invJSON string
	err := d.queryRow(`SELECT jid, nome, coins, xp, level, daily, wins, losses, achievements, inventory, spouse, married_at, created_at FROM users WHERE jid = ?`, jid).
		Scan(&u.JID, &u.Nome, &u.Coins, &u.XP, &u.Level, &u.Daily, &u.Wins, &u.Losses, &achJSON, &invJSON, &u.Spouse, &u.MarriedAt, &u.CreatedAt)
	if err == sql.ErrNoRows {
		now := time.Now().UnixMilli()
		u = User{
			JID:          jid,
			Level:        1,
			Achievements: []string{},
			Inventory:    []string{},
			CreatedAt:    now,
		}
		_, _ = d.exec(`INSERT INTO users (jid, level, created_at) VALUES (?, 1, ?) ON CONFLICT(jid) DO NOTHING`, jid, now)
		return &u
	} else if err != nil {
		return &User{JID: jid, Level: 1, Achievements: []string{}, Inventory: []string{}}
	}
	_ = json.Unmarshal([]byte(achJSON), &u.Achievements)
	_ = json.Unmarshal([]byte(invJSON), &u.Inventory)
	if u.Achievements == nil {
		u.Achievements = []string{}
	}
	if u.Inventory == nil {
		u.Inventory = []string{}
	}
	return &u
}

func (d *DB) SaveUser(u *User) error {
	u.JID = normJID(u.JID)
	achJSON, _ := json.Marshal(u.Achievements)
	invJSON, _ := json.Marshal(u.Inventory)
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO users (jid, nome, coins, xp, level, daily, wins, losses, achievements, inventory, spouse, married_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET
			nome=excluded.nome, coins=excluded.coins, xp=excluded.xp, level=excluded.level,
			daily=excluded.daily, wins=excluded.wins, losses=excluded.losses,
			achievements=excluded.achievements, inventory=excluded.inventory,
			spouse=excluded.spouse, married_at=excluded.married_at`,
		u.JID, u.Nome, u.Coins, u.XP, u.Level, u.Daily, u.Wins, u.Losses,
		string(achJSON), string(invJSON), u.Spouse, u.MarriedAt, u.CreatedAt)
	return err
}

func XPForLevel(level int64, base int64, mult float64) int64 {
	if level <= 1 {
		return base
	}
	return int64(math.Round(float64(base) * math.Pow(mult, float64(level-1))))
}

func (d *DB) AddXP(jid string, amount int64, base int64, mult float64) (leveled bool, newLevel int64) {
	u := d.GetUser(jid)
	u.XP += amount
	for {
		need := XPForLevel(u.Level, base, mult)
		if u.XP >= need {
			u.XP -= need
			u.Level++
			leveled = true
		} else {
			break
		}
	}
	_ = d.SaveUser(u)
	return leveled, u.Level
}

func (d *DB) AddCoins(jid string, amount int64) int64 {
	u := d.GetUser(jid)
	u.Coins += amount
	if u.Coins < 0 {
		u.Coins = 0
	}
	_ = d.SaveUser(u)
	return u.Coins
}

func (d *DB) TopUsers(orderBy string, limit int) []*User {
	if orderBy != "coins" && orderBy != "level" && orderBy != "wins" {
		orderBy = "level"
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.query(fmt.Sprintf(`SELECT jid, nome, coins, xp, level, wins, losses FROM users ORDER BY %s DESC, xp DESC LIMIT ?`, orderBy), limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.JID, &u.Nome, &u.Coins, &u.XP, &u.Level, &u.Wins, &u.Losses); err == nil {
			out = append(out, u)
		}
	}
	return out
}

// Donos & Premium
func (d *DB) IsDonoExtra(jid string) bool {
	jid = normJID(jid)
	d.mu.RLock()
	defer d.mu.RUnlock()
	var count int
	_ = d.queryRow(`SELECT COUNT(1) FROM donos WHERE jid = ?`, jid).Scan(&count)
	return count > 0
}

func (d *DB) AddDono(jid string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO donos (jid) VALUES (?) ON CONFLICT(jid) DO NOTHING`, normJID(jid))
	return err
}

func (d *DB) DelDono(jid string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`DELETE FROM donos WHERE jid = ?`, normJID(jid))
	return err
}

func (d *DB) ListDonos() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.query(`SELECT jid FROM donos`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var j string
		if rows.Scan(&j) == nil {
			out = append(out, j)
		}
	}
	return out
}

func (d *DB) IsPremium(jid string) bool {
	jid = normJID(jid)
	d.mu.RLock()
	defer d.mu.RUnlock()
	var exp int64
	err := d.queryRow(`SELECT expires_at FROM premium_users WHERE jid = ?`, jid).Scan(&exp)
	if err != nil {
		return false
	}
	if exp > 0 && exp < time.Now().UnixMilli() {
		return false
	}
	return true
}

func (d *DB) AddPremium(jid string, days int) error {
	var exp int64
	if days > 0 {
		exp = time.Now().Add(time.Duration(days) * 24 * time.Hour).UnixMilli()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO premium_users (jid, expires_at) VALUES (?, ?) ON CONFLICT(jid) DO UPDATE SET expires_at=excluded.expires_at`, normJID(jid), exp)
	return err
}

func (d *DB) DelPremium(jid string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`DELETE FROM premium_users WHERE jid = ?`, normJID(jid))
	return err
}

func (d *DB) ListPremium() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.query(`SELECT jid, expires_at FROM premium_users`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	now := time.Now().UnixMilli()
	var out []string
	for rows.Next() {
		var j string
		var exp int64
		if rows.Scan(&j, &exp) == nil {
			if exp == 0 || exp > now {
				out = append(out, j)
			}
		}
	}
	return out
}

// Group Features
func (d *DB) GetGroupFeature(groupID, feature string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var en int
	err := d.queryRow(`SELECT enabled FROM group_features WHERE group_id = ? AND feature = ?`, groupID, feature).Scan(&en)
	if err != nil {
		if feature == "antidelete" || feature == "viewonce" {
			return true
		}
		return false
	}
	return en == 1
}

func (d *DB) SetGroupFeature(groupID, feature string, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO group_features (group_id, feature, enabled) VALUES (?, ?, ?)
		ON CONFLICT(group_id, feature) DO UPDATE SET enabled=excluded.enabled`, groupID, feature, val)
	return err
}

func (d *DB) GetGroupPrefix(groupID string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var p string
	_ = d.queryRow(`SELECT prefix FROM group_prefixes WHERE group_id = ?`, groupID).Scan(&p)
	return p
}

func (d *DB) SetGroupPrefix(groupID, prefix string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if prefix == "" {
		_, err := d.exec(`DELETE FROM group_prefixes WHERE group_id = ?`, groupID)
		return err
	}
	_, err := d.exec(`INSERT INTO group_prefixes (group_id, prefix) VALUES (?, ?) ON CONFLICT(group_id) DO UPDATE SET prefix=excluded.prefix`, groupID, prefix)
	return err
}

// Active Groups (Aluguel)
func (d *DB) IsGroupActive(groupID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var exp int64
	err := d.queryRow(`SELECT expires_at FROM active_groups WHERE group_id = ?`, groupID).Scan(&exp)
	if err == sql.ErrNoRows {
		// Fallback seguro igual ao original: se não existe no banco, permite
		return true
	}
	if err != nil {
		return true
	}
	if exp == 0 {
		return true
	}
	return exp > time.Now().UnixMilli()
}

func (d *DB) GetGroupExpiry(groupID string) (exists bool, expiresAt int64) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	err := d.queryRow(`SELECT expires_at FROM active_groups WHERE group_id = ?`, groupID).Scan(&expiresAt)
	if err != nil {
		return false, 0
	}
	return true, expiresAt
}

func (d *DB) ActivateGroup(groupID string, days int, by string) error {
	exp := int64(0)
	if days > 0 {
		exp = time.Now().Add(time.Duration(days) * 24 * time.Hour).UnixMilli()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO active_groups (group_id, expires_at, activated_by) VALUES (?, ?, ?)
		ON CONFLICT(group_id) DO UPDATE SET expires_at=excluded.expires_at, activated_by=excluded.activated_by`, groupID, exp, by)
	return err
}

func (d *DB) DeactivateGroup(groupID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO active_groups (group_id, expires_at, activated_by) VALUES (?, 1, 'disabled')
		ON CONFLICT(group_id) DO UPDATE SET expires_at=1`, groupID)
	return err
}

// Boas-vindas
func (d *DB) IsWelcomeEnabled(groupID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var ativo int
	err := d.queryRow(`SELECT ativo FROM boas_vindas WHERE group_id = ?`, groupID).Scan(&ativo)
	return err == nil && ativo == 1
}

func (d *DB) SetWelcomeEnabled(groupID string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO boas_vindas (group_id, ativo) VALUES (?, ?) ON CONFLICT(group_id) DO UPDATE SET ativo=excluded.ativo`, groupID, v)
	return err
}

// Mutes
func (d *DB) IsMuted(groupID, userJID string) bool {
	userJID = normJID(userJID)
	d.mu.RLock()
	var until int64
	err := d.queryRow(`SELECT until_ts FROM mutes WHERE group_id = ? AND user_jid = ?`, groupID, userJID).Scan(&until)
	d.mu.RUnlock()
	if err != nil {
		return false
	}
	if until > 0 && until < time.Now().UnixMilli() {
		_ = d.RemoveMute(groupID, userJID)
		return false
	}
	return true
}

func (d *DB) SetMute(groupID, userJID string, untilTS int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO mutes (group_id, user_jid, until_ts) VALUES (?, ?, ?) ON CONFLICT(group_id, user_jid) DO UPDATE SET until_ts=excluded.until_ts`, groupID, normJID(userJID), untilTS)
	return err
}

func (d *DB) RemoveMute(groupID, userJID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`DELETE FROM mutes WHERE group_id = ? AND user_jid = ?`, groupID, normJID(userJID))
	return err
}

func (d *DB) ListMutes(groupID string) map[string]int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.query(`SELECT user_jid, until_ts FROM mutes WHERE group_id = ?`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var j string
		var u int64
		if rows.Scan(&j, &u) == nil {
			out[j] = u
		}
	}
	return out
}

// Warns
func (d *DB) AddWarn(groupID, userJID, reason string) int {
	userJID = normJID(userJID)
	d.mu.Lock()
	defer d.mu.Unlock()
	var count int
	var rJSON string
	err := d.queryRow(`SELECT count, reasons FROM warns WHERE group_id = ? AND user_jid = ?`, groupID, userJID).Scan(&count, &rJSON)
	var reasons []string
	if err == nil {
		_ = json.Unmarshal([]byte(rJSON), &reasons)
	}
	count++
	if reason != "" {
		reasons = append(reasons, reason)
	}
	b, _ := json.Marshal(reasons)
	_, _ = d.exec(`INSERT INTO warns (group_id, user_jid, count, reasons) VALUES (?, ?, ?, ?)
		ON CONFLICT(group_id, user_jid) DO UPDATE SET count=excluded.count, reasons=excluded.reasons`, groupID, userJID, count, string(b))
	return count
}

func (d *DB) RemoveWarn(groupID, userJID string) int {
	userJID = normJID(userJID)
	d.mu.Lock()
	defer d.mu.Unlock()
	var count int
	err := d.queryRow(`SELECT count FROM warns WHERE group_id = ? AND user_jid = ?`, groupID, userJID).Scan(&count)
	if err != nil || count <= 0 {
		return 0
	}
	count--
	if count == 0 {
		_, _ = d.exec(`DELETE FROM warns WHERE group_id = ? AND user_jid = ?`, groupID, userJID)
	} else {
		_, _ = d.exec(`UPDATE warns SET count = ? WHERE group_id = ? AND user_jid = ?`, count, groupID, userJID)
	}
	return count
}

func (d *DB) ListWarns(groupID string) map[string]WarnEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.query(`SELECT user_jid, count, reasons FROM warns WHERE group_id = ?`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make(map[string]WarnEntry)
	for rows.Next() {
		var j, rJSON string
		var c int
		if rows.Scan(&j, &c, &rJSON) == nil {
			var reasons []string
			_ = json.Unmarshal([]byte(rJSON), &reasons)
			out[j] = WarnEntry{Count: c, Reasons: reasons}
		}
	}
	return out
}

// AFK
func (d *DB) SetAFK(userJID, reason string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO afk (user_jid, reason, since_ts) VALUES (?, ?, ?) ON CONFLICT(user_jid) DO UPDATE SET reason=excluded.reason, since_ts=excluded.since_ts`,
		normJID(userJID), reason, time.Now().UnixMilli())
	return err
}

func (d *DB) GetAFK(userJID string) *AFKEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var e AFKEntry
	err := d.queryRow(`SELECT reason, since_ts FROM afk WHERE user_jid = ?`, normJID(userJID)).Scan(&e.Reason, &e.SinceTS)
	if err != nil {
		return nil
	}
	return &e
}

func (d *DB) ClearAFK(userJID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`DELETE FROM afk WHERE user_jid = ?`, normJID(userJID))
	return err
}

// Blacklist
func (d *DB) AddBlacklist(groupID, userJID, reason, by string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO blacklist (group_id, user_jid, reason, added_by, added_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(group_id, user_jid) DO UPDATE SET reason=excluded.reason`, groupID, normJID(userJID), reason, by, time.Now().UnixMilli())
	return err
}

func (d *DB) RemoveBlacklist(groupID, userJID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`DELETE FROM blacklist WHERE group_id = ? AND user_jid = ?`, groupID, normJID(userJID))
	return err
}

func (d *DB) IsBlacklisted(groupID, userJID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var c int
	_ = d.queryRow(`SELECT COUNT(1) FROM blacklist WHERE (group_id = ? OR group_id = '*') AND user_jid = ?`, groupID, normJID(userJID)).Scan(&c)
	return c > 0
}

func (d *DB) ListBlacklist(groupID string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.query(`SELECT user_jid FROM blacklist WHERE group_id = ?`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var j string
		if rows.Scan(&j) == nil {
			out = append(out, j)
		}
	}
	return out
}

// Autoreply
func (d *DB) SetAutoreply(groupID, trigger, response string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO autoreply (group_id, trigger_word, response) VALUES (?, ?, ?)
		ON CONFLICT(group_id, trigger_word) DO UPDATE SET response=excluded.response`, groupID, strings.ToLower(trigger), response)
	return err
}

func (d *DB) DelAutoreply(groupID, trigger string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`DELETE FROM autoreply WHERE group_id = ? AND trigger_word = ?`, groupID, strings.ToLower(trigger))
	return err
}

func (d *DB) GetAutoreplies(groupID string) map[string]string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.query(`SELECT trigger_word, response FROM autoreply WHERE group_id = ?`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var t, r string
		if rows.Scan(&t, &r) == nil {
			out[t] = r
		}
	}
	return out
}

// Agendamentos Fechar
func (d *DB) SetScheduledClose(groupID, horario string, minutosAbrir int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO agendamentos_fechar (group_id, horario_fechar, minutos_abrir) VALUES (?, ?, ?)
		ON CONFLICT(group_id) DO UPDATE SET horario_fechar=excluded.horario_fechar, minutos_abrir=excluded.minutos_abrir`, groupID, horario, minutosAbrir)
	return err
}

func (d *DB) DeleteScheduledClose(groupID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`DELETE FROM agendamentos_fechar WHERE group_id = ?`, groupID)
	return err
}

func (d *DB) GetScheduledCloses(horario string) []ScheduledClose {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.query(`SELECT group_id, horario_fechar, minutos_abrir FROM agendamentos_fechar WHERE horario_fechar = ?`, horario)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []ScheduledClose
	for rows.Next() {
		var s ScheduledClose
		if rows.Scan(&s.GroupID, &s.HorarioFechar, &s.MinutosAbrir) == nil {
			out = append(out, s)
		}
	}
	return out
}

// Marriage proposals
func (d *DB) SetProposal(fromJID, toJID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`INSERT INTO marriage_proposals (to_jid, from_jid, created_at) VALUES (?, ?, ?)
		ON CONFLICT(to_jid) DO UPDATE SET from_jid=excluded.from_jid, created_at=excluded.created_at`,
		normJID(toJID), normJID(fromJID), time.Now().UnixMilli())
	return err
}

func (d *DB) GetProposal(toJID string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var from string
	_ = d.queryRow(`SELECT from_jid FROM marriage_proposals WHERE to_jid = ?`, normJID(toJID)).Scan(&from)
	return from
}

func (d *DB) ClearProposal(toJID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.exec(`DELETE FROM marriage_proposals WHERE to_jid = ?`, normJID(toJID))
	return err
}
