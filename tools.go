package main

// registerTools sets up the available tools for the agent
func (a *Agent) registerTools() {
	registerSearchReplaceTool(a)
	registerListFilesTool(a)
	registerReadFileTool(a)
	registerWriteFileTool(a)
	registerRipgrepTool(a)
	registerGoDocTool(a)
	registerGoVetTool(a)
}
