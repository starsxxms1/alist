package _89

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/Xhofe/alist/conf"
	"github.com/Xhofe/alist/drivers/base"
	"github.com/Xhofe/alist/model"
	"github.com/Xhofe/alist/utils"
	"github.com/go-resty/resty/v2"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
	mathRand "math/rand"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var client189Map map[string]*resty.Client

func (driver Cloud189) FormatFile(file *Cloud189File) *model.File {
	f := &model.File{
		Id:        strconv.FormatInt(file.Id, 10),
		Name:      file.Name,
		Size:      file.Size,
		Driver:    driver.Config().Name,
		UpdatedAt: nil,
		Thumbnail: file.Icon.SmallUrl,
		Url:       file.Url,
	}
	loc, _ := time.LoadLocation("Local")
	lastOpTime, err := time.ParseInLocation("2006-01-02 15:04:05", file.LastOpTime, loc)
	if err == nil {
		f.UpdatedAt = &lastOpTime
	}
	if file.Size == -1 {
		f.Type = conf.FOLDER
		f.Size = 0
	} else {
		f.Type = utils.GetFileType(filepath.Ext(file.Name))
	}
	return f
}

//func (c Cloud189) GetFile(path string, account *model.Account) (*Cloud189File, error) {
//	dir, name := filepath.Split(path)
//	dir = utils.ParsePath(dir)
//	_, _, err := c.ParentPath(dir, account)
//	if err != nil {
//		return nil, err
//	}
//	parentFiles_, _ := conf.Cache.Get(conf.Ctx, fmt.Sprintf("%s%s", account.Name, dir))
//	parentFiles, _ := parentFiles_.([]Cloud189File)
//	for _, file := range parentFiles {
//		if file.Name == name {
//			if file.Size != -1 {
//				return &file, err
//			} else {
//				return nil, ErrNotFile
//			}
//		}
//	}
//	return nil, ErrPathNotFound
//}

type Cloud189Down struct {
	ResCode         int    `json:"res_code"`
	ResMessage      string `json:"res_message"`
	FileDownloadUrl string `json:"fileDownloadUrl"`
}

type LoginResp struct {
	Msg    string `json:"msg"`
	Result int    `json:"result"`
	ToUrl  string `json:"toUrl"`
}

// Login refer to PanIndex
func (driver Cloud189) Login(account *model.Account) error {
	client, ok := client189Map[account.Name]
	if !ok {
		//cookieJar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		client = resty.New()
		//client.SetCookieJar(cookieJar)
		client.SetRetryCount(3)
		client.SetHeader("Referer", "https://cloud.189.cn/")
	}
	url := "https://cloud.189.cn/api/portal/loginUrl.action?redirectURL=https%3A%2F%2Fcloud.189.cn%2Fmain.action"
	b := ""
	lt := ""
	ltText := regexp.MustCompile(`lt = "(.+?)"`)
	for i := 0; i < 3; i++ {
		res, err := client.R().Get(url)
		if err != nil {
			return err
		}
		b = res.String()
		ltTextArr := ltText.FindStringSubmatch(b)
		if len(ltTextArr) > 0 {
			lt = ltTextArr[1]
			break
		} else {
			<-time.After(time.Second)
		}
	}
	if lt == "" {
		return fmt.Errorf("get empty login page")
	}
	captchaToken := regexp.MustCompile(`captchaToken' value='(.+?)'`).FindStringSubmatch(b)[1]
	returnUrl := regexp.MustCompile(`returnUrl = '(.+?)'`).FindStringSubmatch(b)[1]
	paramId := regexp.MustCompile(`paramId = "(.+?)"`).FindStringSubmatch(b)[1]
	//reqId := regexp.MustCompile(`reqId = "(.+?)"`).FindStringSubmatch(b)[1]
	jRsakey := regexp.MustCompile(`j_rsaKey" value="(\S+)"`).FindStringSubmatch(b)[1]
	vCodeID := regexp.MustCompile(`picCaptcha\.do\?token\=([A-Za-z0-9\&\=]+)`).FindStringSubmatch(b)[1]
	vCodeRS := ""
	if vCodeID != "" {
		// need ValidateCode
	}
	userRsa := RsaEncode([]byte(account.Username), jRsakey)
	passwordRsa := RsaEncode([]byte(account.Password), jRsakey)
	url = "https://open.e.189.cn/api/logbox/oauth2/loginSubmit.do"
	var loginResp LoginResp
	res, err := client.R().
		SetHeaders(map[string]string{
			"lt":         lt,
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36",
			"Referer":    "https://open.e.189.cn/",
			"accept":     "application/json;charset=UTF-8",
		}).SetFormData(map[string]string{
		"appKey":       "cloud",
		"accountType":  "01",
		"userName":     "{RSA}" + userRsa,
		"password":     "{RSA}" + passwordRsa,
		"validateCode": vCodeRS,
		"captchaToken": captchaToken,
		"returnUrl":    returnUrl,
		"mailSuffix":   "@pan.cn",
		"paramId":      paramId,
		"clientType":   "10010",
		"dynamicCheck": "FALSE",
		"cb_SaveName":  "1",
		"isOauth2":     "false",
	}).Post(url)
	if err != nil {
		return err
	}
	err = utils.Json.Unmarshal(res.Body(), &loginResp)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	if loginResp.Result != 0 {
		return fmt.Errorf(loginResp.Msg)
	}
	_, err = client.R().Get(loginResp.ToUrl)
	if err != nil {
		log.Errorf(err.Error())
		return err
	}
	client189Map[account.Name] = client
	return nil
}

type Cloud189Error struct {
	ErrorCode string `json:"errorCode"`
	ErrorMsg  string `json:"errorMsg"`
}

type Cloud189File struct {
	Id         int64  `json:"id"`
	LastOpTime string `json:"lastOpTime"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Icon       struct {
		SmallUrl string `json:"smallUrl"`
		//LargeUrl string `json:"largeUrl"`
	} `json:"icon"`
	Url string `json:"url"`
}

type Cloud189Folder struct {
	Id         int64  `json:"id"`
	LastOpTime string `json:"lastOpTime"`
	Name       string `json:"name"`
}

type Cloud189Files struct {
	ResCode    int    `json:"res_code"`
	ResMessage string `json:"res_message"`
	FileListAO struct {
		Count      int              `json:"count"`
		FileList   []Cloud189File   `json:"fileList"`
		FolderList []Cloud189Folder `json:"folderList"`
	} `json:"fileListAO"`
}

func (driver Cloud189) GetFiles(fileId string, account *model.Account) ([]Cloud189File, error) {
	client, ok := client189Map[account.Name]
	if !ok {
		return nil, fmt.Errorf("can't find [%s] client", account.Name)
	}
	res := make([]Cloud189File, 0)
	pageNum := 1
	for {
		var e Cloud189Error
		var resp Cloud189Files
		_, err := client.R().SetResult(&resp).SetError(&e).
			SetHeader("Accept", "application/json;charset=UTF-8").
			SetQueryParams(map[string]string{
				"noCache":    random(),
				"pageSize":   "60",
				"pageNum":    strconv.Itoa(pageNum),
				"mediaType":  "0",
				"folderId":   fileId,
				"iconOption": "5",
				"orderBy":    account.OrderBy,
				"descending": account.OrderDirection,
			}).Get("https://cloud.189.cn/api/open/file/listFiles.action")
		if err != nil {
			return nil, err
		}
		if e.ErrorCode != "" {
			if e.ErrorCode == "InvalidSessionKey" {
				err = driver.Login(account)
				if err != nil {
					return nil, err
				}
				return driver.GetFiles(fileId, account)
			}
		}
		if resp.ResCode != 0 {
			return nil, fmt.Errorf(resp.ResMessage)
		}
		if resp.FileListAO.Count == 0 {
			break
		}
		for _, folder := range resp.FileListAO.FolderList {
			res = append(res, Cloud189File{
				Id:         folder.Id,
				LastOpTime: folder.LastOpTime,
				Name:       folder.Name,
				Size:       -1,
			})
		}
		res = append(res, resp.FileListAO.FileList...)
		pageNum++
	}
	return res, nil
}

func (driver Cloud189) Request(url string, method string, form map[string]string, headers map[string]string, account *model.Account) ([]byte, error) {
	client, ok := client189Map[account.Name]
	if !ok {
		return nil, fmt.Errorf("can't find [%s] client", account.Name)
	}
	//var resp base.Json
	var e Cloud189Error
	req := client.R().SetError(&e).
		SetHeader("Accept", "application/json;charset=UTF-8").
		SetQueryParams(map[string]string{
			"noCache": random(),
		})
	if form != nil {
		req = req.SetFormData(form)
	}
	if headers != nil {
		req = req.SetHeaders(headers)
	}
	var err error
	var res *resty.Response
	if strings.ToUpper(method) == "GET" {
		res, err = req.Get(url)
	} else {
		res, err = req.Post(url)
	}
	if err != nil {
		return nil, err
	}
	if e.ErrorCode != "" {
		if e.ErrorCode == "InvalidSessionKey" {
			err = driver.Login(account)
			if err != nil {
				return nil, err
			}
			return driver.Request(url, method, form, nil, account)
		}
	}
	//log.Debug(res, jsoniter.Get(res.Body(),"res_code").ToInt())
	if jsoniter.Get(res.Body(), "res_code").ToInt() != 0 {
		err = errors.New(jsoniter.Get(res.Body(), "res_message").ToString())
	}
	return res.Body(), err
}

func (driver Cloud189) GetSessionKey(account *model.Account) (string, error) {
	resp, err := driver.Request("https://cloud.189.cn/v2/getUserBriefInfo.action", "GET", nil, nil, account)
	if err != nil {
		return "", err
	}
	return jsoniter.Get(resp, "sessionKey").ToString(), nil
}

func (driver Cloud189) GetResKey(account *model.Account) (string, string, error) {
	resp, err := driver.Request("https://cloud.189.cn/api/security/generateRsaKey.action", "GET", nil, nil, account)
	if err != nil {
		return "", "", err
	}
	return jsoniter.Get(resp, "pubKey").ToString(), jsoniter.Get(resp, "pkId").ToString(), nil
}

func (driver Cloud189) UploadRequest(url string, form map[string]string, account *model.Account) ([]byte, error) {
	sessionKey, err := driver.GetSessionKey(account)
	if err != nil {
		return nil, err
	}
	pubKey, pkId, err := driver.GetResKey(account)
	if err != nil {
		return nil, err
	}
	xRId := "e007e99a-370c-4a14-a143-1b1541972fcf"
	pkey := strings.ReplaceAll(xRId, "-", "")
	params := aesEncrypt(qs(form), pkey[:16])
	date := strconv.FormatInt(time.Now().Unix(), 10)
	signature := hmacSha1(fmt.Sprintf("SessionKey=%s&Operate=GET&RequestURI=%s&Date=%s&params=%s", sessionKey, url, date, params), pkey)
	encryptionText := RsaEncode([]byte(pkey), pubKey)
	res, err := base.RestyClient.R().SetHeaders(map[string]string{
		"signature":      signature,
		"sessionKey":     sessionKey,
		"encryptionText": encryptionText,
		"pkId":           pkId,
		"x-request-id":   xRId,
		"x-request-date": date,
		"origin":         "https://cloud.189.cn",
		"referer":        "https://cloud.189.cn/",
	}).SetQueryParam("params", params).Get("https://upload.cloud.189.cn" + url)
	if err != nil {
		return nil, err
	}
	log.Debug(res.String())
	data := res.Body()
	if jsoniter.Get(data, "code").ToString() != "SUCCESS" {
		return nil, errors.New(jsoniter.Get(data, "msg").ToString())
	}
	return data, nil
}

func random() string {
	return fmt.Sprintf("0.%17v", mathRand.New(mathRand.NewSource(time.Now().UnixNano())).Int63n(100000000000000000))
}

func RsaEncode(origData []byte, j_rsakey string) string {
	publicKey := []byte("-----BEGIN PUBLIC KEY-----\n" + j_rsakey + "\n-----END PUBLIC KEY-----")
	block, _ := pem.Decode(publicKey)
	pubInterface, _ := x509.ParsePKIXPublicKey(block.Bytes)
	pub := pubInterface.(*rsa.PublicKey)
	b, err := rsa.EncryptPKCS1v15(rand.Reader, pub, origData)
	if err != nil {
		log.Errorf("err: %s", err.Error())
	}
	return b64tohex(base64.StdEncoding.EncodeToString(b))
}

var b64map = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

var BI_RM = "0123456789abcdefghijklmnopqrstuvwxyz"

func int2char(a int) string {
	return strings.Split(BI_RM, "")[a]
}

func b64tohex(a string) string {
	d := ""
	e := 0
	c := 0
	for i := 0; i < len(a); i++ {
		m := strings.Split(a, "")[i]
		if m != "=" {
			v := strings.Index(b64map, m)
			if 0 == e {
				e = 1
				d += int2char(v >> 2)
				c = 3 & v
			} else if 1 == e {
				e = 2
				d += int2char(c<<2 | v>>4)
				c = 15 & v
			} else if 2 == e {
				e = 3
				d += int2char(c)
				d += int2char(v >> 2)
				c = 3 & v
			} else {
				e = 0
				d += int2char(c<<2 | v>>4)
				d += int2char(15 & v)
			}
		}
	}
	if e == 1 {
		d += int2char(c << 2)
	}
	return d
}

func qs(form map[string]string) string {
	strList := make([]string, 0)
	for k, v := range form {
		strList = append(strList, fmt.Sprintf("%s=%s", k, url.QueryEscape(v)))
	}
	return strings.Join(strList, "&")
}

func aesEncrypt(data, key string) string {
	encrypted := AesEncryptECB([]byte(data), []byte(key))
	//return string(encrypted)
	return hex.EncodeToString(encrypted)
}

func hmacSha1(data string, secret string) string {
	h := hmac.New(sha1.New, []byte(secret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func AesEncryptECB(origData []byte, key []byte) (encrypted []byte) {
	cipher, _ := aes.NewCipher(generateKey(key))
	length := (len(origData) + aes.BlockSize) / aes.BlockSize
	plain := make([]byte, length*aes.BlockSize)
	copy(plain, origData)
	pad := byte(len(plain) - len(origData))
	for i := len(origData); i < len(plain); i++ {
		plain[i] = pad
	}
	encrypted = make([]byte, len(plain))
	// 分组分块加密
	for bs, be := 0, cipher.BlockSize(); bs <= len(origData); bs, be = bs+cipher.BlockSize(), be+cipher.BlockSize() {
		cipher.Encrypt(encrypted[bs:be], plain[bs:be])
	}

	return encrypted
}
func AesDecryptECB(encrypted []byte, key []byte) (decrypted []byte) {
	cipher, _ := aes.NewCipher(generateKey(key))
	decrypted = make([]byte, len(encrypted))
	//
	for bs, be := 0, cipher.BlockSize(); bs < len(encrypted); bs, be = bs+cipher.BlockSize(), be+cipher.BlockSize() {
		cipher.Decrypt(decrypted[bs:be], encrypted[bs:be])
	}

	trim := 0
	if len(decrypted) > 0 {
		trim = len(decrypted) - int(decrypted[len(decrypted)-1])
	}

	return decrypted[:trim]
}
func generateKey(key []byte) (genKey []byte) {
	genKey = make([]byte, 16)
	copy(genKey, key)
	for i := 16; i < len(key); {
		for j := 0; j < 16 && i < len(key); j, i = j+1, i+1 {
			genKey[j] ^= key[i]
		}
	}
	return genKey
}

func getMd5(data []byte) []byte {
	h := md5.New()
	h.Write(data)
	return h.Sum(nil)
}

func init() {
	base.RegisterDriver(&Cloud189{})
	client189Map = make(map[string]*resty.Client, 0)
}
