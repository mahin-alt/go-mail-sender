package main

import "fmt"

func main() {
	config, err := loadConfig()
	if err != nil {
		panic(err)
	}

	fmt.Println("SMTP configuration loaded")
	fmt.Println("Host:", config.Host)
	fmt.Println("Port:", config.Port)
	fmt.Println("Username:", config.Username)

	err = sendMail(config)
	if err != nil {
		panic(err)
	}

	fmt.Println("email success snet")
}
