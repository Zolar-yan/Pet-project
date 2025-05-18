package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// Структура задачи
type Task struct {
	ID          int
	Description string
	Completed   bool
}

var tasks []Task
var nextID = 1
var bot *tgbotapi.BotAPI

// Структура состояния пользователя
type UserState struct {
	AwaitingDescription bool
	AwaitingDoneID      bool // новое состояние для /done
}

var userStates = make(map[int64]*UserState)

// Функции для работы с задачами
func addTask(description string) Task {
	task := Task{ID: nextID, Description: description, Completed: false}
	tasks = append(tasks, task)
	nextID++
	return task
}

func completeTask(id int) bool {
	for i, task := range tasks {
		if task.ID == id {
			if tasks[i].Completed {
				return false // уже выполнена
			}
			tasks[i].Completed = true
			return true
		}
	}
	return false // не найдено
}

func getTasks() []Task {
	return tasks
}

// Обработка команд и логика бота

func handleStartCommand() string {
	return "Привет! Я ваш бот-планировщик. Выберите действие ниже или используйте /help для списка команд."
}

func handleHelpCommand() string {
	return `Доступные команды:
/start - начать работу с ботом
/add - добавить новую задачу (после команды нужно ввести описание)
/list - показать список задач
/done - отметить задачу как выполненную по ID (после команды нужно ввести ID)`
}

func handleAddCommand(description string) string {
	task := addTask(description)
	return fmt.Sprintf("✅ Задача добавлена: %s (ID: %d)", task.Description, task.ID)
}

func handleCompleteCommand(id int) string {
	if completeTask(id) {
		return fmt.Sprintf("✅ Задача с ID %d помечена как выполненная.", id)
	}
	return fmt.Sprintf("❌ Задача с ID %d не найдена или уже выполнена.", id)
}

func handleListCommand() string {
	var response strings.Builder
	for _, task := range getTasks() {
		var statusEmoji string
		if task.Completed {
			statusEmoji = "✅"
		} else {
			statusEmoji = "❌"
		}
		response.WriteString(fmt.Sprintf("ID: %d - %s [%s]\n", task.ID, task.Description, statusEmoji))
	}
	if response.Len() == 0 {
		return "Список задач пуст."
	}
	return response.String()
}

// Создаем клавиатуру для меню с командами
func createKeyboard() tgbotapi.ReplyKeyboardMarkup {
	buttons := [][]tgbotapi.KeyboardButton{
		{
			tgbotapi.NewKeyboardButton("Добавить задачу"),
			tgbotapi.NewKeyboardButton("Список задач"),
			tgbotapi.NewKeyboardButton("/help"),
			tgbotapi.NewKeyboardButton("Выполнено"),
		},
	}
	return tgbotapi.NewReplyKeyboard(buttons...)
}

func main() {

	// Чтение токена из файла bot_token.txt в той же папке, что и main.go
	tokenBytes, err := ioutil.ReadFile("bot_token.txt")
	if err != nil {
		log.Panicf("Ошибка при чтении файла токена: %v", err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatal(err)
	}

	for update := range updates {
		if update.Message == nil { // игнорируем не сообщения обновления
			continue
		}

		chatID := update.Message.Chat.ID

		// Проверяем состояние пользователя перед обработкой команд или сообщений
		if state, exists := userStates[chatID]; exists && state.AwaitingDescription {
			// Пользователь вводит описание новой задачи
			description := update.Message.Text
			handleAddCommand(description)
			delete(userStates, chatID)

			// Отправляем подтверждение пользователю и показываем меню снова
			msg := tgbotapi.NewMessage(chatID, "Задача добавлена.")
			msg.ReplyMarkup = createKeyboard()
			bot.Send(msg)
			continue

		} else if exists && state.AwaitingDoneID { // новая проверка для /done по ID
			idStr := strings.TrimSpace(update.Message.Text)
			id, err := strconv.Atoi(idStr)
			if err != nil || id <= 0 {
				msg := tgbotapi.NewMessage(chatID, "Некорректный ID. Пожалуйста, введите положительное число.")
				bot.Send(msg)
				continue
			}
			response := handleCompleteCommand(id)
			msg := tgbotapi.NewMessage(chatID, response)
			msg.ReplyMarkup = createKeyboard()
			bot.Send(msg)

			// После обработки снимаем состояние ожидания ID для /done
			delete(userStates, chatID)
			continue

		}

		text := update.Message.Text

		switch text {

		case "/start", "Главное меню":
			response := handleStartCommand()
			keyboard := createKeyboard()
			msg := tgbotapi.NewMessage(chatID, response)
			msg.ReplyMarkup = keyboard
			bot.Send(msg)

		case "Добавить задачу":
			userStates[chatID] = &UserState{AwaitingDescription: true}
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, введите описание задачи:")
			bot.Send(msg)

		case "Список задач":
			response := handleListCommand()
			msg := tgbotapi.NewMessage(chatID, response)
			bot.Send(msg)

		case "/help":
			response := handleHelpCommand()
			msg := tgbotapi.NewMessage(chatID, response)
			bot.Send(msg)

		case "Выполнено":
			// Устанавливаем состояние ожидания ввода ID задачи для этого пользователя.
			userStates[chatID] = &UserState{AwaitingDoneID: true}
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, введите ID задачи для завершения:")
			bot.Send(msg)

		default:
			// Обработка команд /add и /list при вводе текста (если пользователь их вводит вручную)
			if strings.HasPrefix(text, "/add") {
				parts := strings.SplitN(text, " ", 2)
				if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
					msg := tgbotapi.NewMessage(chatID, "Пожалуйста, укажите описание задачи после команды /add.")
					bot.Send(msg)
				} else {
					description := strings.TrimSpace(parts[1])
					response := handleAddCommand(description)
					msg := tgbotapi.NewMessage(chatID, response)
					bot.Send(msg)
				}
				continue
			}

			if strings.HasPrefix(text, "/list") || text == "/list" {
				response := handleListCommand()
				msg := tgbotapi.NewMessage(chatID, response)
				bot.Send(msg)
				continue
			}

			// Если сообщение не распознано — выводим подсказку или меню снова.
			msg := tgbotapi.NewMessage(chatID, "Неизвестная команда. Пожалуйста, выберите действие из меню или используйте /help.")
			bot.Send(msg)

		}
	}
}
