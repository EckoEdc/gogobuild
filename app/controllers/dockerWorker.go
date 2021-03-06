package controllers

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/revel/revel"
)

//DockerWorker Controller implementing Worker interface
type DockerWorker struct {
	docker           *docker.Client
	build            Build
	targetSys        string
	imageName        string
	logFile          *os.File
	outputDir        string
	commitToFallback bool
}

func (d *DockerWorker) init() error {
	var err error
	d.docker, err = docker.NewClient("unix:///var/run/docker.sock")

	return err
}

//Run the DockerWorker
func (d DockerWorker) Run() {
	var err error

	d.build.StartDate = time.Now()
	BMInstance().UpdateBuild(&d.build)
	//Create log file
	d.outputDir = fmt.Sprintf("%s/public/output/%s/%d/%s", revel.BasePath, d.build.ProjectToBuild.Name, d.build.Date.Unix(), d.build.TargetSys)
	os.MkdirAll(d.outputDir, 0777)
	d.logFile, err = os.Create(d.outputDir + "/logs.txt")

	err = d.init()
	if err != nil {
		d.logFile.WriteString(err.Error())
		return
	}
	d.imageName = fmt.Sprintf("gogobuild/%s_%s:", d.build.ProjectToBuild.Name, strings.ToLower(d.targetSys)) + "%s"

	//Check if the fallback image exists else it's the first time we need to build it
	_, err = d.docker.InspectImage(fmt.Sprintf(d.imageName, "fallback"))
	if err == docker.ErrNoSuchImage {
		d.logFile.WriteString("No image found, try to build it.")
		d.buildImage()
		return
	}
	if err != nil {
		d.logFile.WriteString(err.Error())
		d.build.State = Fail
		BMInstance().UpdateBuild(&d.build)
		return
	}

	d.startBuild()
	return
}

//buildImage build an image from
func (d *DockerWorker) buildImage() error {

	t := time.Now()
	inputbuf, outputbuf := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	file, err := os.Open(revel.BasePath + "/public/projects/" + d.build.ProjectToBuild.Name + "/docker/" + d.build.TargetSys + "/Dockerfile")
	if err != nil {
		d.logFile.WriteString(err.Error())
		d.build.State = Fail
		BMInstance().UpdateBuild(&d.build)
		return err
	}
	defer file.Close()
	fileContent, err := ioutil.ReadAll(file)
	if err != nil {
		d.logFile.WriteString(err.Error())
		d.build.State = Fail
		BMInstance().UpdateBuild(&d.build)
		return err
	}
	tr := tar.NewWriter(inputbuf)
	tr.WriteHeader(&tar.Header{Name: "Dockerfile", Size: int64(len(fileContent)), ModTime: t, AccessTime: t, ChangeTime: t})
	tr.Write(fileContent)
	tr.Close()

	opts := docker.BuildImageOptions{
		Name:           fmt.Sprintf(d.imageName, "fallback"),
		RmTmpContainer: true,
		InputStream:    inputbuf,
		OutputStream:   outputbuf,
		NoCache:        true,
	}
	if err := d.docker.BuildImage(opts); err != nil {
		d.logFile.WriteString(err.Error())
		d.build.State = Fail
		BMInstance().UpdateBuild(&d.build)
	} else {
		d.logFile.Write(outputbuf.Bytes())
		d.startBuild()
	}

	return nil
}

//Build the docker image
func (d *DockerWorker) startBuild() error {

	defer d.logFile.Close()

	if d.build.Commit == "updateWorker" {
		d.commitToFallback = true
	}

	d.build.State = Init
	BMInstance().UpdateBuild(&d.build)

	var err error
	//Use fallback image for refs/build
	useFallbackImage := d.build.Commit != "master"
	if useFallbackImage == false || d.commitToFallback == true {
		//try to make an up to date image
		err = d.tryUpdate()
		if err != nil {
			useFallbackImage = true
			d.logFile.WriteString("\n\n---Update Image failed falling back---\n")
		} else {
			d.logFile.WriteString("\n\n---Using updated image---\n")
		}
	}

	//Don't build the project if that's an update build
	if d.commitToFallback == false {
		//build the project
		err = d.buildProject(useFallbackImage)
		if err != nil && useFallbackImage == false {
			//Last chance to make it work
			d.logFile.WriteString("\n\n---Build with updated image failed, falling back...---\n")
			useFallbackImage = true
			err = d.buildProject(useFallbackImage)
		}
	}
	//Set the build final state
	if err != nil {
		d.build.State = Fail
	} else {
		if useFallbackImage && d.commitToFallback == false {
			d.build.State = FallbackSuccess
		} else {
			d.build.State = Success
		}
	}
	BMInstance().UpdateBuild(&d.build)
	return nil
}

func (d *DockerWorker) tryUpdate() error {
	//Start time
	start := time.Now().Round(time.Second)

	var cmds []string
	cmds = append(cmds, "bash")
	cmds = append(cmds, "-c")
	cmds = append(cmds, strings.Join(d.build.ProjectToBuild.Configuration.UpdateInstructions[d.targetSys], " && "), d.build.Commit)
	d.logFile.WriteString(strings.Join(cmds, "\n"))
	d.logFile.WriteString("\n\n ---UPDATE OUTPUT---- \n")

	config := &docker.Config{
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
		Tty:          false,
		Cmd:          cmds,
		Image:        fmt.Sprintf(d.imageName, "fallback"),
	}
	hostConfig := &docker.HostConfig{Binds: []string{d.outputDir + ":/output"}}
	containerConfig := docker.CreateContainerOptions{
		Config:     config,
		HostConfig: hostConfig,
	}
	container, err := d.docker.CreateContainer(containerConfig)
	if err != nil {
		d.logFile.WriteString("\n" + err.Error())
		log.Println(err)
		return err
	}

	logOptions := docker.LogsOptions{
		Stdout:       true,
		Stderr:       true,
		Timestamps:   true,
		Container:    container.ID,
		OutputStream: d.logFile,
		ErrorStream:  d.logFile,
	}

	err = d.docker.StartContainer(container.ID, hostConfig)
	if err != nil {
		d.logFile.WriteString("\n" + err.Error())
		log.Println(err)
		return err
	}
	//Wait for the container
	retValue, err := d.docker.WaitContainer(container.ID)

	errLog := d.docker.Logs(logOptions)
	if errLog != nil {
		log.Println(errLog.Error())
	}

	//Set the update duration even if it failed
	d.build.UpdateWorkerDuration = time.Now().Round(time.Second).Sub(start)
	BMInstance().UpdateBuild(&d.build)

	if retValue != 0 || err != nil {
		d.destroy(container.ID)
		return errors.New("Update Failed")
	}
	err = d.docker.RemoveImage(fmt.Sprintf(d.imageName, "latest"))
	if err != nil && err != docker.ErrNoSuchImage {
		return err
	}
	suffix := "latest"
	if d.commitToFallback == true {
		suffix = "fallback"
	}
	_, err = d.docker.CommitContainer(docker.CommitContainerOptions{Container: container.ID, Repository: fmt.Sprintf("gogobuild/%s_%s", d.build.ProjectToBuild.Name, strings.ToLower(d.targetSys)), Tag: suffix})
	d.destroy(container.ID)
	return err
}

//UpdateOrFallback to good docker image
func (d *DockerWorker) buildProject(fallBack bool) error {

	var cmds []string
	cmds = append(cmds, "bash")
	cmds = append(cmds, "-c")

	re := regexp.MustCompile("{{REF_NUMBER}}")
	releaseNumber := regexp.MustCompile("{{RELEASE_NUMBER}}")
	releaseString := d.build.Date.Format("20060102150400") + "~git" + d.build.GitCommitID

	cmds = append(cmds, releaseNumber.ReplaceAllString(
		re.ReplaceAllString(strings.Join(d.build.ProjectToBuild.Configuration.BuildInstructions[d.targetSys], " && "), d.build.Commit),
		releaseString))
	d.logFile.WriteString(strings.Join(cmds, "\n"))
	d.logFile.WriteString("\n\n ---OUTPUT---- \n")

	var suffix string
	if fallBack == true {
		suffix = "fallback"
	} else {
		suffix = "latest"
	}

	config := &docker.Config{
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
		Tty:          false,
		Cmd:          cmds,
		Image:        fmt.Sprintf(d.imageName, suffix),
	}
	sourceDir := fmt.Sprintf("%s/%s/%s:/%s", revel.BasePath, "public/projects/", d.build.ProjectToBuild.Name, d.build.ProjectToBuild.Name)
	hostConfig := &docker.HostConfig{Binds: []string{d.outputDir + ":/output", sourceDir}}
	containerConfig := docker.CreateContainerOptions{
		Config:     config,
		HostConfig: hostConfig,
	}
	container, err := d.docker.CreateContainer(containerConfig)
	if err != nil {
		d.logFile.WriteString("\n" + err.Error())
		log.Println(err)
		return err
	}

	logOptions := docker.LogsOptions{
		Stdout:       true,
		Stderr:       true,
		Timestamps:   true,
		Container:    container.ID,
		OutputStream: d.logFile,
		ErrorStream:  d.logFile,
	}

	// Start the container
	err = d.docker.StartContainer(container.ID, hostConfig)
	if err != nil {
		d.logFile.WriteString("\n" + err.Error())
		log.Println(err)
		return err
	}

	d.build.State = Building
	BMInstance().UpdateBuild(&d.build)

	//Wait for the container to do it's work
	retValue, err := d.docker.WaitContainer(container.ID)

	//Copy the log to the logFile
	errLog := d.docker.Logs(logOptions)
	if errLog != nil {
		log.Println(errLog.Error())
	}

	//Remove the container
	d.destroy(container.ID)

	if err != nil || retValue != 0 {
		d.logFile.WriteString("\nBUILD FAILED\n")
		return errors.New("Build failed")
	}
	d.logFile.WriteString("\nBUILD SUCCESS\n")
	return nil
}

//Destroy docker image
func (d *DockerWorker) destroy(containerID string) {
	d.docker.RemoveContainer(docker.RemoveContainerOptions{ID: containerID, Force: true, RemoveVolumes: false})
}
