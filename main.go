package main

import (
	"dcbot/bot"
	"dcbot/config"
	"dcbot/store"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("config error: ", err)
	}

	st, err := store.Load()
	if err != nil {
		log.Fatal("store error: ", err)
	}

	b, err := bot.New(cfg, st)
	if err != nil {
		log.Fatal("bot init error: ", err)
	}

	if err := b.Start(); err != nil {
		log.Fatal("bot start error: ", err)
	}
	defer b.Stop()

	log.Println("Bot is running. Press CTRL+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
	log.Println("Shutting down...")
}
