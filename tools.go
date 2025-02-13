package main

// registerTools sets up the available tools for the agent
func (a *Agent) registerTools() {
	registerListFilesTool(a)
	registerReadFileTool(a)
	registerWriteFileTool(a)
}