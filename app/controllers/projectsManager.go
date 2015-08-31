package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/revel/modules/jobs/app/jobs"
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
	AutoDeploySchedule map[string]string
	DeployScript       string
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
	err := p.loadConf(dir.Name())
	if err == nil {
		for sys, time := range p.Configuration.AutoDeploySchedule {
			buildInstr := p.Configuration.BuildInstructions[sys]
			if buildInstr != nil {
				//Explicitly capture sys
				targetSys := sys
				jobs.Schedule(time, jobs.Func(func() {
					BMInstance().CreateOrReturnStatusBuild(p.Name, targetSys, "master", true)
				}))
			}
		}

	}
	return err
}

//Reload configuration
func (p *Project) Reload() error {
	for _, instr := range p.Configuration.ReloadProjectCmd {
		cmd := exec.Command(instr)
		cmd.Dir = revel.BasePath + "/public/projects/" + p.Name
		cmd.Run()
	}
	return p.loadConf(p.Name)
}

//GetHeadCommitID return the head commit id
func (p *Project) GetHeadCommitID() string {
	gitDir := fmt.Sprintf("--git-dir=%s/public/projects/%s/.git", revel.BasePath, p.Name)
	exec.Command("git", "fetch", "origin")
	cmd := exec.Command("git", gitDir, "rev-parse", "--short=7", "origin/master")
	ref, _ := cmd.CombinedOutput()
	return strings.Replace(string(ref), "\n", "", -1)
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
