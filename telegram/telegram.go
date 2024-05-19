package telegram

import (
	log "log/slog"
	"net/url"
	"os"
	"strconv"
	"sync"

	telegram "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/quangnguyen/registrator/bridge"
)

var registeredServices sync.Map

func init() {
	bridge.Register(new(Factory), "telegram")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	bot, err := telegram.NewBotAPI(botToken)
	if err != nil {
		log.Error("Failed to create Telegram bot", "error", err)
	}

	chatID, err := strconv.ParseInt(uri.Host, 10, 64)
	if err != nil {
		log.Error("Invalid chat ID", "error", err)
	}

	return &Telegram{bot: bot, chatID: chatID}
}

type Telegram struct {
	bot    *telegram.BotAPI
	chatID int64
}

func (t *Telegram) Ping() error {
	bot, err := t.bot.GetMe()
	if err != nil {
		return err
	}

	log.Debug("Ping telegram bot", "bot", bot)
	return nil
}

func (t *Telegram) Register(service *bridge.Service) error {
	err := t.sendMessage("ONLINE: Service " + service.Name + " with ip " + service.IP + " goes online")
	if err == nil {
		registeredServices.Store(service.Name, service.ID)
	}
	return err
}

func (t *Telegram) Deregister(service *bridge.Service) error {
	if _, serviceID := registeredServices.LoadAndDelete(service.Name); serviceID {
		err := t.sendMessage("OFFLINE: Service " + service.Name + " with ip " + service.IP + " goes offline")
		return err
	}
	return nil
}

func (t *Telegram) Refresh(_ *bridge.Service) error {
	return nil
}

func (t *Telegram) Services() ([]*bridge.Service, error) {
	var services []*bridge.Service
	registeredServices.Range(func(serviceName, serviceID interface{}) bool {
		service := &bridge.Service{ID: serviceName.(string)}
		services = append(services, service)
		return true
	})
	return services, nil
}

func (t *Telegram) sendMessage(text string) error {
	msg := telegram.NewMessage(t.chatID, text)
	_, err := t.bot.Send(msg)
	if err != nil {
		log.Error("Could not send message to Telegram", "error", err)
		return err
	}
	return nil
}
