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
	"strings"
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
	REST_BASE_PATH string
}

type SftpServer struct {
	sftpServerAddress string
	sftpServerPort string
	sftpUserName string
	sftpUserPassword string
	restBasePath string
	conn *ssh.Client
	client *sftp.Client
}

func InitServer(cfg ServerCfg) (**SftpServer, error){
	server := &SftpServer{
		sftpServerAddress: cfg.SFTP_SERVER_ADDRESS,
		sftpServerPort:    cfg.SFTP_SERVER_PORT,
		sftpUserName:      cfg.SFTP_USER_NAME,
		sftpUserPassword:  cfg.SFTP_USER_PASSWORD,
		restBasePath:cfg.REST_BASE_PATH,
	}

	config := &ssh.ClientConfig{
		User: server.sftpUserName,
		Auth: []ssh.AuthMethod{
			ssh.Password(server.sftpUserPassword),
		},
		HostKeyCallback:ssh.InsecureIgnoreHostKey(),
	}

	log.Info("address: ", server.sftpServerAddress+":"+server.sftpServerPort)
	conn, err := ssh.Dial("tcp", server.sftpServerAddress+":"+server.sftpServerPort, config)
	if err != nil {
		log.WithError(err).Error("SSh dial failed")
		return nil, err
	}
	server.conn = conn

	client, err := sftp.NewClient(conn)
	if err != nil {
		log.WithError(err).Error("create sftp failed")
		return nil, err
	}
	server.client = client

	return &server, nil
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
	path = s.getFolder(path)
	log.Info("Folder:", path)
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
	path = s.getFolder(path)
	log.Info("Folder:", path)
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
	path = s.getFolder(path)
	log.Info("Folder:", path)
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
	path = s.getFolder(path)
	log.Info("Folder:", path)
	info, err := s.client.Lstat(path)
	if err != nil {
		log.WithError(err).Error("Get file info error")
		tmpErr := Wrap(err, "Get file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	srcFile, err := s.client.Open(path)
	if err != nil {
		log.WithError(err).Error("Get file error")
		tmpErr := Wrap(err, "Get file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	var headerBuffSize int64
	if info.Size() < 512 {
		headerBuffSize = info.Size()
	}else {
		headerBuffSize = 512
	}

	fileHeader := make([]byte, headerBuffSize)
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
	path = s.getFolder(path)
	log.Info("Folder:", path)
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
	path = s.getFolder(path)
	log.Info("Folder:", path)
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

func (s *SftpServer)getFolder(path string) string {
	return strings.Replace(path, s.restBasePath, "", 1)
}