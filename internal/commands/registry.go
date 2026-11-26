package commands

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	mu        sync.RWMutex
	commands  map[string]*Command
	aliases   map[string]string
	cooldowns map[string]time.Time
}

func NewRegistry() *Registry {
	return &Registry{
		commands:  make(map[string]*Command),
		aliases:   make(map[string]string),
		cooldowns: make(map[string]time.Time),
	}
}

func (r *Registry) Register(cmd *Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.ToLower(cmd.Name)
	r.commands[key] = cmd
	for _, a := range cmd.Aliases {
		ak := strings.ToLower(a)
		if _, exists := r.commands[ak]; !exists {
			r.aliases[ak] = key
		}
	}
}

func (r *Registry) Get(name string) *Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := strings.ToLower(name)
	if c, ok := r.commands[key]; ok {
		return c
	}
	if orig, ok := r.aliases[key]; ok {
		return r.commands[orig]
	}
	return nil
}

func (r *Registry) AllUnique() []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Command, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) Categories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]bool)
	for _, c := range r.commands {
		if c.Category != "" {
			seen[c.Category] = true
		}
	}
	cats := make([]string, 0, len(seen))
	for k := range seen {
		cats = append(cats, k)
	}
	sort.Strings(cats)
	return cats
}

func (r *Registry) ListByCategory(cat string) []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Command
	for _, c := range r.commands {
		if strings.EqualFold(c.Category, cat) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) CanRun(cmd *Command, ctx *Context) (bool, string) {
	if cmd.OnlyPv && ctx.IsGroup {
		return false, "🔒 Este comando só pode ser usado no privado."
	}
	if cmd.OnlyGroup && !ctx.IsGroup {
		return false, "🔒 Este comando só pode ser usado em grupos."
	}
	if cmd.Dono && !ctx.IsDono {
		return false, "🔒 Apenas o dono pode usar este comando."
	}
	if cmd.Admin && !ctx.IsAdmin && !ctx.IsDono {
		return false, "🔒 Apenas administradores."
	}
	if cmd.NeedBotAdmin && !ctx.IsBotAdmin {
		return false, "🤖 Preciso ser admin do grupo."
	}
	if cmd.Premium && !ctx.IsVip && !ctx.IsDono {
		return false, "💎 Comando exclusivo para premium."
	}

	cd := cmd.Cooldown
	if cd <= 0 {
		cd = ctx.Config.CooldownPadrao
	}
	if !ctx.IsDono && cd > 0 {
		ck := fmt.Sprintf("%s:%s", ctx.Sender.String(), cmd.Name)
		r.mu.Lock()
		until, ok := r.cooldowns[ck]
		now := time.Now()
		if ok && now.Before(until) {
			rem := int(until.Sub(now).Seconds()) + 1
			r.mu.Unlock()
			return false, fmt.Sprintf("⏳ Aguarde *%ds* para usar *%s%s* novamente.", rem, ctx.Prefix, cmd.Name)
		}
		r.cooldowns[ck] = now.Add(time.Duration(cd) * time.Second)
		r.mu.Unlock()
	}
	return true, ""
}

func RegisterAll(r *Registry) {
	registerAdmin(r)
	registerDono(r)
	registerUtilidades(r)
	registerJogos(r)
	registerMidias(r)
	registerDownloads(r)
	registerExtras(r)
	registerResenha(r)
}
