package controllers

import "github.com/revel/revel"

func init() {
	revel.OnAppStart(func() {
		BMInstance().BuildMaintenance()
	})
}

//App struct
type App struct {
	*revel.Controller
}

//Index only redirect to projects index
func (c App) Index() revel.Result {
	return c.Redirect(ProjectsController.Index)
}
