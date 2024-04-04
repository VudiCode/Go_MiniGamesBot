package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
)

const (
	Rock     = iota // камень
	Scissors = iota // ножницы
	Paper    = iota // бумага

	Join = iota // присоединиться
	Exit = iota // выйти

	Victory = iota // победа
	Defeat  = iota // поражение
	Draw    = iota // ничья
)

const (
	telegramAPIURL = "https://api.telegram.org/bot"
	botToken       = ""
)

type Update struct {
	UpdateID int     `json:"update_id"`
	Message  Message `json:"message"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

type User struct {
	tableID        int
	currentWins    int
	totalWins      int
	currentDefeats int
	totalDefeats   int
	ID             int
}

type UserUpdate struct {
	win  int
	lose int
	text string
}

type PlayingTable struct {
	user      User
	inputChan chan int
	userChan  chan UserUpdate
}

type Chat struct {
	ID int `json:"id"`
}

func main() {
	log.Println("Server started")

	updateChannel := make(chan Update)

	RPSTables := make(map[int]PlayingTable)
	usersTable := make(map[int]User)

	commandTable := make(map[string]int)
	commandTable["/join"] = Join
	commandTable["/exit"] = Exit
	commandTable["/rock"] = Rock
	commandTable["/scissors"] = Scissors
	commandTable["/paper"] = Paper

	increment := 1

	go startPolling(updateChannel)

	for {
		resultText := ""
		var chatID int

		select {
		case update := <-updateChannel:

			chatID = update.Message.Chat.ID

			command, found := commandTable[update.Message.Text]
			if found {
				user, userFound := usersTable[update.Message.Chat.ID]

				if !userFound {
					user = User{}
					user.ID = update.Message.Chat.ID
				}

				table, tableFound := RPSTables[user.tableID]

				if command == Join {

					if user.tableID > 0 {
						resultText = "Вы уже находитесь за столом " + strconv.Itoa(user.tableID)
					} else {
						table = PlayingTable{
							user:      user,
							inputChan: make(chan int),
							userChan:  make(chan UserUpdate),
						}

						user.tableID = increment

						RPSTables[increment] = table

						go operatingTable(table.inputChan, table.userChan)

						resultText = "Теперь вы сидите за столом " + strconv.Itoa(increment)
						increment++
					}
				} else if command == Exit && user.tableID > 0 {
					resultText = "Вы вышли из-за стола " + strconv.Itoa(user.tableID)

					user.tableID = 0
					user.currentWins = 0
					user.currentDefeats = 0

					table.inputChan <- command

					delete(RPSTables, user.tableID)
				} else if tableFound {
					table.inputChan <- command
				} else {
					resultText = "Вы не находитесь ни за одним из столов."
				}

				usersTable[update.Message.Chat.ID] = user
			} else {
				resultText = "Данная команда не существует"
			}

			log.Printf("[%s] input:%s output:%s", update.Message.Chat.ID, update.Message.Text, resultText)
		default:
			for _, table := range RPSTables {
				select {
				case userUpdate := <-table.userChan:
					user, _ := usersTable[table.user.ID]

					chatID = user.ID

					user.totalWins += userUpdate.win
					user.currentWins += userUpdate.win
					user.totalDefeats += userUpdate.lose
					user.currentDefeats += userUpdate.lose

					resultText = userUpdate.text
				default:
					continue
				}
			}
		}

		if resultText != "" {
			err := sendMessage(chatID, resultText)
			if err != nil {
				return
			}
		}
	}
}

func operatingTable(tableUpdateChannel <-chan int, userUpdateChannel chan UserUpdate) {
	for {
		select {
		case data := <-tableUpdateChannel:
			if data == Exit {
				return
			} else {
				battleResult := getResultRPS(data)
				resultText := ""

				userUpdate := UserUpdate{}

				switch {
				case battleResult == Draw:
					resultText = "Ничья"
				case battleResult == Victory:
					userUpdate.win++

					resultText = "Победа"
				case battleResult == Defeat:
					userUpdate.lose++

					resultText = "Поражение"
				}

				userUpdate.text = resultText
				userUpdateChannel <- userUpdate
			}
		}
	}
}

func getResultRPS(userChoice int) int {

	compChoice := rand.Intn(3)

	switch {
	case userChoice == compChoice:
		return Draw
	case userChoice == Rock && compChoice == Scissors ||
		userChoice == Scissors && compChoice == Paper ||
		userChoice == Paper && compChoice == Rock:
		return Victory
	default:
		return Defeat
	}
}

func startPolling(updateChannel chan<- Update) {
	offset := 0
	for {
		updates, err := getUpdates(offset)
		if err != nil {
			log.Println("Ошибка при получении обновлений:", err)
			continue
		}

		for _, update := range updates {
			updateChannel <- update
			offset = update.UpdateID + 1
		}
	}
}

func getUpdates(offset int) ([]Update, error) {
	url := telegramAPIURL + botToken + "/getUpdates?offset=" + strconv.Itoa(offset)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if !response.OK {
		return nil, errors.New("не удалось получить обновления")
	}

	return response.Result, nil
}

func sendMessage(chatID int, text string) error {
	url := telegramAPIURL + botToken + "/sendMessage"

	message := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return errors.New("не удалось отправить сообщение")
	}

	return nil
}
