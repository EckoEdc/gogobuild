package controllers

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	"github.com/revel/revel"
)

//ProjectConfiguration struct
type ProjectConfiguration struct {
	BuildType          string
	BuildInstructions  map[string][]string
	UpdateInstructions map[string][]string
	ReviewType         string
	ReviewAddress      string
	Package            map[string]string
	ReloadProjectCmd   []string
}

//ReviewManager interface
type ReviewManager interface {
	Init(p *Project)
	GetOpenChanges() ([]string, error)
}

//Project struct
type Project struct {
	Name                  string
	Configuration         ProjectConfiguration
	ReviewManagerInstance ReviewManager `bson:"-"`
}

//Init configuration
func (p *Project) Init(dir os.FileInfo) error {
	return p.loadConf(dir.Name())
}

//Reload configuration
func (p *Project) Reload() error {
	//TODO: we should git pull and reset before that to ensure configuration is up to date
	for _, instr := range p.Configuration.ReloadProjectCmd {
		cmd := exec.Command(instr)
		cmd.Dir = revel.BasePath + "/public/projects/" + p.Name
		cmd.Run()
	}
	return p.loadConf(p.Name)
}

//loadConf load the json conf (e.g .packer.json)
func (p *Project) loadConf(fileName string) error {
	file, err := os.Open(revel.BasePath + "/public/projects/" + fileName + "/.packer.json")
	if err != nil {
		log.Println(err)
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	p.Configuration = ProjectConfiguration{}
	err = decoder.Decode(&p.Configuration)
	if err != nil {
		log.Println(err)
		return err
	}

	switch p.Configuration.ReviewType {
	case "Gerrit":
		p.ReviewManagerInstance = new(GerritManager)
		p.ReviewManagerInstance.Init(p)
	}
	return nil
}

//ProjectsManager represent the project we want to compile
type ProjectsManager struct {
	projects map[string]Project
}

//instance of ProjectsManager
var pmInstance *ProjectsManager

//PMInstance Return the instance
func PMInstance() *ProjectsManager {
	if pmInstance == nil {
		pmInstance = new(ProjectsManager)
		pmInstance.init()
	}
	return pmInstance
}

//init all the projects in public/project directory
func (pm *ProjectsManager) init() error {
	dirInfo, err := ioutil.ReadDir(revel.BasePath + "/public/projects")
	if err != nil {
		return err
	}
	pm.projects = make(map[string]Project)
	for _, dir := range dirInfo {
		if dir.IsDir() == true {
			proj := Project{Name: dir.Name()}
			if proj.Init(dir) == nil {
				pm.projects[dir.Name()] = proj
			}
		}
	}
	return nil
}

//GetProjectsList return list of projects
func (pm *ProjectsManager) GetProjectsList() []Project {
	values := make([]Project, 0, len(pm.projects))
	for _, p := range pm.projects {
		values = append(values, p)
	}
	return values
}

//GetProjectByName return a project by name
func (pm *ProjectsManager) GetProjectByName(name string) Project {
	return pm.projects[name]
}
