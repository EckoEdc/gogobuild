package controllers

import (
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/smtp"

	"github.com/revel/revel"
)

//MailManager is the mail manager
type MailManager struct {
}

//instance of MailManager
var mmInstance *MailManager

//MMInstance Return the instance of mailer manager
func MMInstance() *MailManager {
	if mmInstance == nil {
		mmInstance = new(MailManager)
	}
	return mmInstance
}

//SendBuildFailedMail send a mail in case of failed build
func (m *MailManager) SendBuildFailedMail(b Build) {

	smtpServer := revel.Config.StringDefault("mail.smtp", "")
	if len(smtpServer) == 0 {
		revel.WARN.Println("No SMTP server configured in app.conf")
		return
	}
	from := mail.Address{
		Name:    revel.Config.StringDefault("mail.name", ""),
		Address: revel.Config.StringDefault("mail.addr", "")}
	if len(b.ProjectToBuild.Configuration.NotificationMailAdress) < 2 {
		revel.WARN.Println("NotificationMailAdress mail adress not or mis-configured")
		return
	}
	to := mail.Address{
		Name:    b.ProjectToBuild.Configuration.NotificationMailAdress[0],
		Address: b.ProjectToBuild.Configuration.NotificationMailAdress[1]}

	title := fmt.Sprintf("GoGo Build %s/%s Failed", b.ProjectToBuild.Name, b.TargetSys)
	servAddr := revel.Config.StringDefault("http.addr", "")
	if len(servAddr) == 0 {
		revel.WARN.Println("http.addr not configured in app.conf")
		return
	}
	port := revel.Config.StringDefault("http.port", "")
	if len(port) > 0 {
		port = ":" + port
	}
	//TODO: Use template to allow more flexibility
	body := fmt.Sprintf("Build <a href='http://%s%s/projects/%s/builds/%s'>%s</a> of project %s for sys %s has failed.",
		servAddr,
		port,
		b.ProjectToBuild.Name,
		b.ID.Hex(),
		b.ID.Hex(),
		b.ProjectToBuild.Name,
		b.TargetSys)

	header := make(map[string]string)
	header["From"] = from.String()
	header["To"] = to.String()
	header["Subject"] = title
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=\"UTF-8\""
	header["Content-Transfer-Encoding"] = "base64"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(body))

	err := smtp.SendMail(
		smtpServer+":25",
		nil,
		from.Address,
		[]string{to.Address},
		[]byte(message),
	)
	if err != nil {
		revel.WARN.Println(err)
	}
}
