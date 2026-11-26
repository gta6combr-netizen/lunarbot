package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"nyxbot-go/internal/bot"
	"nyxbot-go/internal/commands"
	"nyxbot-go/internal/config"
	"nyxbot-go/internal/db"
	"nyxbot-go/internal/logger"

	"go.mau.fi/whatsmeow/types"
)

func main() {
	_ = os.Setenv("TZ", "America/Sao_Paulo")
	if loc, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		time.Local = loc
	}

	rootDir, err := os.Getwd()
	if err != nil {
		logger.Red("Erro ao obter diretório atual: %v", err)
		os.Exit(1)
	}

	_ = os.MkdirAll(filepath.Join(rootDir, "temp"), 0755)
	_ = os.MkdirAll(filepath.Join(rootDir, "database"), 0755)

	cfg, err := config.Load(rootDir)
	if err != nil {
		logger.Red("Erro ao carregar config: %v", err)
		os.Exit(1)
	}

	database, err := db.Open(filepath.Join(rootDir, "database"))
	if err != nil {
		logger.Red("Erro ao abrir SQLite: %v", err)
		os.Exit(1)
	}
	defer database.Close()

	reg := commands.NewRegistry()
	commands.RegisterAll(reg)
	logger.Cyan("🩸 %s (Go) iniciando — %d comandos registrados", cfg.NomeDoBot, len(reg.AllUnique()))

	useCode := slices.Contains(os.Args[1:], "--code")
	if useCode {
		logger.Cyan("📱 Modo pairing code ativado.")
	} else {
		logger.Cyan("📷 Modo QR Code ativado.")
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	client, err := bot.NewClient(cfg, database, reg, useCode, func() {
		stopCh <- syscall.SIGTERM
	})
	if err != nil {
		logger.Red("Erro ao iniciar cliente WhatsApp: %v", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	// Temp cleanup goroutine
	go func() {
		maxAge := time.Duration(cfg.TempCleanupMinutes) * time.Minute
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			entries, err := os.ReadDir(filepath.Join(rootDir, "temp"))
			if err != nil {
				continue
			}
			now := time.Now()
			for _, e := range entries {
				if info, err := e.Info(); err == nil {
					if now.Sub(info.ModTime()) > maxAge {
						_ = os.Remove(filepath.Join(rootDir, "temp", e.Name()))
					}
				}
			}
		}
	}()

	// Scheduled group close checker
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			horario := time.Now().Format("15:04")
			rows := database.GetScheduledCloses(horario)
			for _, row := range rows {
				gid, err := types.ParseJID(row.GroupID)
				if err != nil {
					continue
				}
				logger.Cyan("[Agendamento] Fechando grupo %s (%s)", row.GroupID, horario)
				_ = client.Raw().SetGroupAnnounce(context.Background(), gid, true)
				_ = client.Raw().SetGroupLocked(context.Background(), gid, true)
				_ = client.SendText(gid, fmt.Sprintf("🔒 *GRUPO FECHADO!*\n🕒 Horário agendado: %s\n⏳ Use %sabrir para liberar.", horario, cfg.Prefix))
				_ = database.DeleteScheduledClose(row.GroupID)
			}
		}
	}()

	// HTTP server for healthcheck & QR code
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	mux.Handle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir(filepath.Join(rootDir, "public")))))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","message":"Lunar Bot Go Online"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(rootDir, "public", "qr.html"))
	})
	mux.HandleFunc("/api/qr-status", func(w http.ResponseWriter, r *http.Request) {
		conn, qr := client.QRStatus()
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"status": "waiting", "qr": nil}
		if conn {
			resp["status"] = "connected"
		} else if len(qr) > 0 {
			resp["status"] = "ready"
			resp["qr"] = true
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/qr-image", func(w http.ResponseWriter, r *http.Request) {
		_, qr := client.QRStatus()
		if len(qr) == 0 {
			http.Error(w, "QR Code não disponível", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(qr)
	})

	srv := &http.Server{Addr: "0.0.0.0:" + port, Handler: mux}
	go func() {
		logger.Green("✅ Servidor HTTP rodando na porta %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Red("HTTP server error: %v", err)
		}
	}()

	<-stopCh
	logger.Yellow("🛑 Encerrando Lunar Bot Go...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
