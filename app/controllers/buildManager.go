package controllers

import (
	"archive/tar"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/revel/revel"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

//State enum
type State int

//State enum *Keep this ordered*
const (
	Created  State = iota //0
	Init                  //1
	Building              //2
	Fail                  //3

	Success         //4
	FallbackSuccess //5
)

func (s State) String() string {
	switch s {
	case Created:
		return "Created"
	case Init:
		return "Init"
	case Building:
		return "Building"
	case Fail:
		return "Fail"
	case Success:
		return "Success"
	case FallbackSuccess:
		return "FallbackSuccess"
	}
	return "Unknown"
}

//Build represent a build
type Build struct {
	ID             bson.ObjectId `bson:"_id,omitempty"`
	Date           time.Time
	LastUpdated    time.Time
	ProjectToBuild Project
	TargetSys      string
	State          State
	Commit         string
}

//IsDownloadable return true if downloadable
func (b *Build) IsDownloadable() bool {
	if b.State > Fail && b.Commit != "updateWorker" {
		return true
	}
	return false
}

//IsRetryable return true if we can retry a failed build
func (b *Build) IsRetryable() bool {
	if b.Commit == "master" {
		return false
	}
	if b.State == Fail {
		return true
	}
	return false
}

//Duration return diff between date and updatedDate
func (b *Build) Duration() time.Duration {
	if b.LastUpdated.IsZero() {
		return 0
	}
	return b.LastUpdated.Sub(b.Date)
}

//BuildManager is the build manager
type BuildManager struct {
	session *mgo.Session
}

//instance of BuildManager
var bmInstance *BuildManager

//BMInstance Return the instance of build manager
func BMInstance() *BuildManager {
	if bmInstance == nil {
		bmInstance = new(BuildManager)
		var err error
		bmInstance.session, err = mgo.Dial("127.0.0.1")
		if err != nil {
			//Pretty much dead if we don't have mongoDB anyway
			log.Fatal(err)
		}
	}
	return bmInstance
}

//CreateOrReturnStatusBuild create or return status of requested build
func (b *BuildManager) CreateOrReturnStatusBuild(projectName string, sys string, commit string) (State, error) {
	if commit == "master" || commit == "updateWorker" {
		return Created, b.NewBuild(projectName, sys, commit)
	}
	c := b.session.DB("gogobuild").C("builds")
	var build = new(Build)
	err := c.Find(bson.M{"projecttobuild.name": projectName, "targetsys": sys, "commit": commit}).One(build)
	if err != nil && err.Error() == "not found" {
		return Created, b.NewBuild(projectName, sys, commit)
	} else if build.State == Fail {
		b.RetryBuild(build)
	}
	return build.State, err
}

//NewBuild create a build and gives it to WorkerManager
func (b *BuildManager) NewBuild(projectName string, sys string, commit string) error {
	project := PMInstance().GetProjectByName(projectName)
	project.Reload()
	build := Build{Date: time.Now(), ProjectToBuild: project, TargetSys: sys, State: Created, Commit: commit}
	WMInstance().Build(&build)
	b.SaveBuild(&build)
	return nil
}

//RetryBuild that failed
func (b *BuildManager) RetryBuild(build *Build) {
	build.ProjectToBuild.Reload()
	WMInstance().Build(build)
}

//GetBuildsByProjects get list of projects builds
func (b *BuildManager) GetBuildsByProjects(projectName string) ([]Build, error) {
	c := b.session.DB("gogobuild").C("builds")
	var buildList []Build
	err := c.Find(bson.M{"projecttobuild.name": projectName}).Sort("-date").All(&buildList)
	if err != nil {
		log.Println(err)
	}
	return buildList, err
}

//GetBuildByID return a build by it's id
func (b *BuildManager) GetBuildByID(id string) (*Build, error) {
	c := b.session.DB("gogobuild").C("builds")
	var build = new(Build)
	err := c.FindId(bson.ObjectIdHex(id)).One(build)
	if err != nil {
		log.Println(err)
	}
	return build, err
}

//UpdateBuild in DB
func (b *BuildManager) UpdateBuild(build *Build) error {
	// TODO: Tar the output/project dir (e.g targetsys all case)
	/*	if build.State > Fail {
		b.PackOutput(build, true)
	}*/
	c := b.session.DB("gogobuild").C("builds")
	err := c.Update(bson.M{"projecttobuild.name": build.ProjectToBuild.Name, "date": build.Date},
		bson.M{"$set": bson.M{"state": build.State, "lastupdated": time.Now()}})
	if err != nil {
		log.Println(err)
	}
	return err
}

//SaveBuild in DB
func (b *BuildManager) SaveBuild(build *Build) error {
	c := b.session.DB("gogobuild").C("builds")
	err := c.Insert(build)
	if err != nil {
		log.Println(err)
	}
	return err
}

//BuildMaintenance should be called in case build were not updated to there final state
//(e.g at start since no build should be in created, init or building state)
func (b *BuildManager) BuildMaintenance() error {
	c := b.session.DB("gogobuild").C("builds")
	_, err := c.UpdateAll(bson.M{"state": bson.M{"$in": []State{Created, Init, Building}}},
		bson.M{"$set": bson.M{"state": Fail}})
	return err
}

//PackOutput : tar the content of the output build directory
func (b *BuildManager) PackOutput(build *Build, excludeLogs bool) error {
	outputDir := fmt.Sprintf("%s/public/output/%s/%d", revel.BasePath, build.ProjectToBuild.Name, build.Date.Unix())
	dir, err := os.Open(outputDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	files, err := dir.Readdir(0) // grab the files list
	if err != nil {
		return err
	}
	tarfile, err := os.Create(fmt.Sprintf("%s/%s%d.tar", outputDir, build.ProjectToBuild.Name, build.Date.Unix()))
	if err != nil {
		return err
	}
	defer tarfile.Close()
	var fileWriter io.WriteCloser = tarfile
	tarfileWriter := tar.NewWriter(fileWriter)
	defer tarfileWriter.Close()
	for _, fileInfo := range files {

		if fileInfo.IsDir() {
			continue
		}

		file, err := os.Open(dir.Name() + string(filepath.Separator) + fileInfo.Name())
		if err != nil {
			return err
		}

		defer file.Close()

		//Don't include logs if we didn't ask for it
		if excludeLogs == true && file.Name() == "logs.txt" {
			continue
		}

		// prepare the tar header
		header := new(tar.Header)
		header.Name = file.Name()
		header.Size = fileInfo.Size()
		header.Mode = int64(fileInfo.Mode())
		header.ModTime = fileInfo.ModTime()

		err = tarfileWriter.WriteHeader(header)
		if err != nil {
			return err
		}

		_, err = io.Copy(tarfileWriter, file)
		if err != nil {
			return err
		}
	}
	return nil
}
