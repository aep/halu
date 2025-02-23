package glad

type Message interface{}

type Request struct {
	Messages []Message
}

type Arg struct {
	Type string
}

type Tool struct {
	Name        string
	Description string
	Args        map[string]Arg
	Required    []string
}

type Callbacks struct {
	Text func(string)
	Tool func(string, map[string]any) string
}

type SessionSetup struct {
	System string
	Tools  []Tool
}
