package rest2sftp

import (
	"fmt"
	"github.com/pkg/sftp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"io"
	"net/http"
	"os"
	"strconv"
)

type FileInfo struct {
	Name string `json:"Name"`
	Size int64 `json:"Size"`
	LastModified string `json:"lastModified`
	IsDirectory bool `json:"isDirectory`
}

type FileInfos struct {
	Files []FileInfo `json:"files"`
}

type ServerCfg struct {
	SFTP_SERVER_ADDRESS string
	SFTP_SERVER_PORT string
	SFTP_USER_NAME string
	SFTP_USER_PASSWORD string
	REST_PORT string
}

type SftpServer struct {
	sftpServerAddress string
	sftpServerPort string
	sftpUserName string
	sftpUserPassword string
	restPort string
	conn *ssh.Client
	client *sftp.Client
}

func NewServer(cfg ServerCfg) *SftpServer{
	return &SftpServer{
		sftpServerAddress: cfg.SFTP_SERVER_ADDRESS,
		sftpServerPort:    cfg.SFTP_SERVER_PORT,
		sftpUserName:      cfg.SFTP_USER_NAME,
		sftpUserPassword:  cfg.SFTP_USER_PASSWORD,
		restPort:cfg.REST_PORT,
	}
}

func (s *SftpServer)Run() error {
	config := &ssh.ClientConfig{
		User: s.sftpUserName,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.sftpUserPassword),
		},
		HostKeyCallback:ssh.InsecureIgnoreHostKey(),
	}

	log.Info("address: ", s.sftpServerAddress+":"+s.sftpServerPort)
	conn, err := ssh.Dial("tcp", s.sftpServerAddress+":"+s.sftpServerPort, config)
	if err != nil {
		log.WithError(err).Fatal("SSh dial failed")
		return err
	}
	s.conn = conn

	client, err := sftp.NewClient(conn)
	if err != nil {
		log.WithError(err).Fatal("create sftp failed")
		return err
	}
	s.client = client

	http.Handle("/", s)
	log.Infof("rest2sftp service is running at port %s", s.restPort)
	return http.ListenAndServe(fmt.Sprintf(":%s", s.restPort), s)
}

func (s *SftpServer)ServeHTTP(w http.ResponseWriter, r *http.Request){
	log.Infof("middleware %s method %s", r.URL, r.Method)
	if r.Method == http.MethodPost {
		if isDirectory(r.URL.Path) {
			s.handlePostFolder(w, r)
		} else {
			s.handlePostFile(w, r)
		}
	}else if r.Method == http.MethodGet {
		if isDirectory(r.URL.Path) {
			s.handleGetFolder(w, r)
		} else {
			s.handleGetFile(w, r)
		}
	}else if r.Method == http.MethodDelete {
		if isDirectory(r.URL.Path){
			s.handleDeleteFolder(w, r)
		}else {
			s.handleDeleteFile(w, r)
		}
	} else {
		fmt.Fprint(w, "Method not allow "+ r.Method)
	}
}

func isDirectory(path string) bool {
	if path[len(path)-1] == '/' {
		return true
	}

	return false
}

func (s *SftpServer)handleGetFolder(w http.ResponseWriter, r *http.Request) {
	log.Info("handle get folder")
	path := r.URL.Path
	fileInfos, err := s.client.ReadDir(path)
	if err != nil {
		log.WithError(err).Error("read directory error")
		tmpErr := Wrap(err, "read directory error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	files := FileInfos{Files: []FileInfo{}}

	for _,fileInfo := range fileInfos {
		files.Files = append(files.Files, convertDto(fileInfo))
	}

	RespondWithJSON(w, http.StatusOK, files)
}

func (s *SftpServer)handlePostFolder(w http.ResponseWriter, r *http.Request) {
	log.Info("handle post folder")
	path := r.URL.Path
	err := s.client.MkdirAll(path)
	if err != nil {
		log.WithError(err).Error("Create directory error")
		tmpErr := Wrap(err, "Create directory error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	RespondNoContent(w, http.StatusOK)
}

func (s *SftpServer)handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	log.Info("handle delete folder")
	path := r.URL.Path
	err := s.client.RemoveDirectory(path)

	if err != nil {
		log.WithError(err).Error("Delete directory error")
		tmpErr := Wrap(err, "Delete directory error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	RespondNoContent(w, http.StatusOK)
}

func (s *SftpServer)handleGetFile(w http.ResponseWriter, r *http.Request) {
	log.Info("handle get file")
	path := r.URL.Path
	srcFile, err := s.client.Open(path)
	if err != nil {
		log.WithError(err).Error("Get file error")
		tmpErr := Wrap(err, "Get file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	fileHeader := make([]byte, 512)
	_, err = srcFile.Read(fileHeader)
	if err != nil {
		log.WithError(err).Error("Get file header error")
		tmpErr := Wrap(err, "Get file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	fileContentType := http.DetectContentType(fileHeader)
	fileStat, err := srcFile.Stat()
	if err != nil {
		log.WithError(err).Error("Get file static error")
		tmpErr := Wrap(err, "Get file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	fileSize := strconv.FormatInt(fileStat.Size(), 10)

	w.Header().Set("Content-Disposition", "attachment; filename=" + path)
	w.Header().Set("Content-Type", fileContentType)
	w.Header().Set("Content-Length", fileSize)

	srcFile.Seek(0, 0)
	_, err = io.Copy(w, srcFile)
	if err != nil {
		log.WithError(err).Error("Copy file error")
		tmpErr := Wrap(err, "Get file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}
}

func (s *SftpServer)handlePostFile(w http.ResponseWriter, r *http.Request) {
	log.Info("handle post file")
	path := r.URL.Path
	r.ParseMultipartForm(6000)
	file, _, err := r.FormFile("file")
	if err != nil {
		log.WithError(err).Error("form upload file error")
		tmpErr := Wrap(err, "Post file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}
	defer file.Close()

	destFile, err := s.client.Create(path)
	if err != nil {
		log.WithError(err).Error("create file error")
		tmpErr := Wrap(err, "Post file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, file)
	if err != nil {
		log.WithError(err).Error("Copy file error")
		tmpErr := Wrap(err, "Post file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	RespondNoContent(w, http.StatusOK)
}

func (s *SftpServer)handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	log.Info("handle delete file")
	path := r.URL.Path
	err := s.client.Remove(path)

	if err != nil {
		log.Info("Delete file error")
		tmpErr := Wrap(err, "Delete file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	RespondNoContent(w, http.StatusOK)
}

func convertDto(info os.FileInfo) FileInfo {
	return FileInfo{
		Name:         info.Name(),
		Size:         info.Size(),
		LastModified: info.ModTime().Format("2006-01-02 15:04:05"),
		IsDirectory:  info.IsDir(),
	}
}
