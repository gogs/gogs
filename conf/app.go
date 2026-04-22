package main

import (
	"fmt"
	"html/template"
	"os"
)

type User struct {
	Name string
}

type ActivationData struct {
	User          User
	ActiveCodeLives int
}

func main() {
	// Configuration
	config := map[string]string{
		"smtp_host":     "smtp.example.com",
		"smtp_port":     "587",
		"smtp_user":     "user@example.com",
		"smtp_password": "password",
	}

	// Email template
	tmpl := template.Must(template.ParseFiles("templates/user/activate_mail.tmpl"))

	// Sample data
	data := ActivationData{
		User:          User{Name: "John Doe"},
		ActiveCodeLives: 120, // Example value
	}

	// Render template
	err := tmpl.Execute(os.Stdout, data)
	if err != nil {
		fmt.Printf("Error executing template: %v\n", err)
		os.Exit(1)
	}
}