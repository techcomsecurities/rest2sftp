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
	"time"
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
}

func InitServer(cfg ServerCfg) **SftpServer{
	server := &SftpServer{
		sftpServerAddress: cfg.SFTP_SERVER_ADDRESS,
		sftpServerPort:    cfg.SFTP_SERVER_PORT,
		sftpUserName:      cfg.SFTP_USER_NAME,
		sftpUserPassword:  cfg.SFTP_USER_PASSWORD,
		restBasePath:cfg.REST_BASE_PATH,
	}

	log.Info("address: ", server.sftpServerAddress+":"+server.sftpServerPort)
	log.Info("Base Path:", server.restBasePath)

	return &server
}

func (s *SftpServer)connect()(*ssh.Client, *sftp.Client, error){
	config := &ssh.ClientConfig{
		User: s.sftpUserName,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.sftpUserPassword),
		},
		HostKeyCallback:ssh.InsecureIgnoreHostKey(),
		Timeout:time.Minute,
	}

	conn, err := ssh.Dial("tcp", s.sftpServerAddress+":"+s.sftpServerPort, config)
	if err != nil {
		log.WithError(err).Error("SSh dial failed")
		return nil, nil, err
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		log.WithError(err).Error("create sftp failed")
		return nil, nil, err
	}

	return conn, client, nil
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

	conn, client, err := s.connect()
	if err != nil {
		log.WithError(err).Error("connect sftp server failed")
		tmpErr := Wrap(err, "connect sftp server failed")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}
	defer conn.Close()
	defer client.Close()

	path := r.URL.Path
	path = s.getFolder(path)
	log.Info("Folder:", path)

	fileInfos, err := client.ReadDir(path)
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

	conn, client, err := s.connect()
	if err != nil {
		log.WithError(err).Error("connect sftp server failed")
		tmpErr := Wrap(err, "connect sftp server failed")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}
	defer conn.Close()
	defer client.Close()

	path := r.URL.Path
	path = s.getFolder(path)
	log.Info("Folder:", path)

	err = client.MkdirAll(path)
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

	conn, client, err := s.connect()
	if err != nil {
		log.WithError(err).Error("connect sftp server failed")
		tmpErr := Wrap(err, "connect sftp server failed")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}
	defer conn.Close()
	defer client.Close()

	path := r.URL.Path
	path = s.getFolder(path)
	log.Info("Folder:", path)
	err = client.RemoveDirectory(path)

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

	conn, client, err := s.connect()
	if err != nil {
		log.WithError(err).Error("connect sftp server failed")
		tmpErr := Wrap(err, "connect sftp server failed")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}
	defer conn.Close()
	defer client.Close()

	path := r.URL.Path
	path = s.getFolder(path)
	log.Info("Folder:", path)
	info, err := client.Lstat(path)
	if err != nil {
		log.WithError(err).Error("Get file info error")
		tmpErr := Wrap(err, "Get file error")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}

	srcFile, err := client.Open(path)
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

	conn, client, err := s.connect()
	if err != nil {
		log.WithError(err).Error("connect sftp server failed")
		tmpErr := Wrap(err, "connect sftp server failed")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}
	defer conn.Close()
	defer client.Close()

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

	destFile, err := client.Create(path)
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

	conn, client, err := s.connect()
	if err != nil {
		log.WithError(err).Error("connect sftp server failed")
		tmpErr := Wrap(err, "connect sftp server failed")
		tmpErr.StatusCode = 1
		RespondWithJSON(w, http.StatusBadRequest, tmpErr)
		return
	}
	defer conn.Close()
	defer client.Close()

	path := r.URL.Path
	path = s.getFolder(path)
	log.Info("Folder:", path)
	err = client.Remove(path)

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