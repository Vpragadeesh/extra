package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/aquasecurity/table"
	"github.com/gen2brain/beeep"
	"os"
	"strconv"
	"strings"
	"time"
)

const todoFile = "todos.json"

// --- Storage ---

type Storage[T any] struct {
	FileName string
}

func NewStorage[T any](fileName string) *Storage[T] {
	return &Storage[T]{FileName: fileName}
}

func (s *Storage[T]) Save(data T) error {
	fileData, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.FileName, fileData, 0644)
}

func (s *Storage[T]) Load(data *T) error {
	fileData, err := os.ReadFile(s.FileName)
	if err != nil {
		return err
	}
	return json.Unmarshal(fileData, data)
}

// --- Todo types and methods ---

type Todo struct {
	Title       string     `json:"title"`
	Completed   bool       `json:"completed"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Todos []Todo

func (todos *Todos) add(title string) {
	todo := Todo{
		Title:       title,
		Completed:   false,
		CompletedAt: nil,
		CreatedAt:   time.Now(),
	}
	*todos = append(*todos, todo)
}

func (todos *Todos) validateIndex(index int) error {
	if index < 0 || index >= len(*todos) {
		return errors.New("invalid index")
	}
	return nil
}

func (todos *Todos) delete(index int) error {
	if err := todos.validateIndex(index); err != nil {
		return err
	}
	*todos = append((*todos)[:index], (*todos)[index+1:]...)
	return nil
}

func (todos *Todos) toggle(index int) error {
	if err := todos.validateIndex(index); err != nil {
		return err
	}
	isCompleted := (*todos)[index].Completed
	if !isCompleted {
		now := time.Now()
		(*todos)[index].CompletedAt = &now
	} else {
		(*todos)[index].CompletedAt = nil
	}
	(*todos)[index].Completed = !isCompleted
	return nil
}

func (todos *Todos) edit(index int, title string) error {
	if err := todos.validateIndex(index); err != nil {
		return err
	}
	(*todos)[index].Title = title
	return nil
}

func (todos *Todos) print() {
	tbl := table.New(os.Stdout)
	tbl.SetRowLines(false)
	tbl.SetHeaders("#", "Title", "Completed", "Created At", "Completed At")
	for i, t := range *todos {
		completed := "❌"
		completedAt := ""
		if t.Completed {
			completed = "✅"
			if t.CompletedAt != nil {
				completedAt = t.CompletedAt.Format(time.RFC1123)
			}
		}
		tbl.AddRow(strconv.Itoa(i), t.Title, completed, t.CreatedAt.Format(time.RFC1123), completedAt)
	}
	tbl.Render()
}

// --- Command-line flags and execution ---

type CmdFlags struct {
	Add    string
	Del    int
	Edit   string
	Toggle int
	List   bool
}

func NewCmdFlags() *CmdFlags {
	cf := CmdFlags{}
	flag.StringVar(&cf.Add, "add", "", "Add a new todo by specifying the title")
	flag.StringVar(&cf.Edit, "edit", "", "Edit a todo by index & specify a new title (format: id:new_title)")
	flag.IntVar(&cf.Del, "del", -1, "Specify a todo by index to delete")
	flag.IntVar(&cf.Toggle, "toggle", -1, "Specify a todo by index to toggle completion")
	flag.BoolVar(&cf.List, "list", false, "List all todos")
	flag.Parse()
	return &cf
}

func (cf *CmdFlags) Execute(todos *Todos) {
	switch {
	case cf.List:
		todos.print()
	case cf.Add != "":
		todos.add(cf.Add)
	case cf.Edit != "":
		parts := strings.SplitN(cf.Edit, ":", 2)
		if len(parts) != 2 {
			fmt.Println("Error: invalid format for edit. Use id:new_title")
			os.Exit(1)
		}
		index, err := strconv.Atoi(parts[0])
		if err != nil {
			fmt.Println("Error: invalid index for edit")
			os.Exit(1)
		}
		if err := todos.edit(index, parts[1]); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case cf.Toggle != -1:
		if err := todos.toggle(cf.Toggle); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case cf.Del != -1:
		if err := todos.delete(cf.Del); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	default:
		fmt.Println("Invalid command")
	}
}

// --- Notification ---

func notifyPendingTodos(todos *Todos) {
	pending := []string{}
	for _, t := range *todos {
		if !t.Completed {
			pending = append(pending, t.Title)
		}
	}
	if len(pending) == 0 {
		return
	}
	message := "Pending todos:\n" + strings.Join(pending, "\n")
	_ = beeep.Notify("Todo Reminder", message, "")
}

// --- Main ---

func main() {
	todos := Todos{}
	storage := NewStorage[Todos](todoFile)

	// Load todos if file exists
	if err := storage.Load(&todos); err != nil && !os.IsNotExist(err) {
		fmt.Println("Error loading todos:", err)
		os.Exit(1)
	}

	cmdFlags := NewCmdFlags()
	cmdFlags.Execute(&todos)

	if err := storage.Save(todos); err != nil {
		fmt.Println("Error saving todos:", err)
		os.Exit(1)
	}

	// Uncomment below to run periodic notifications every 30 minutes
	/*
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			notifyPendingTodos(&todos)
		}
	*/
}
