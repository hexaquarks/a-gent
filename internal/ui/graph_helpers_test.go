package ui

func graphTestModel(count int) Model {
	model := NewModel(nil)
	model.graphVisible = true
	model.sessions = graphTestSessions(count, 5)
	model.width, model.height = 120, 29
	model.resizeTable()
	return model
}
