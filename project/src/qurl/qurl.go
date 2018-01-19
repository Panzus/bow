package qurl

import(
  "strings"
  "strconv"
  "net/http"
  "io/ioutil"
  "crypto/tls"
  "encoding/json"
  "encoding/base64"

  "github.com/Evedel/glb/say"
)

func makequery(rqst *http.Request, secure bool) (body interface{}, header map[string][]string, status int){
  var client *http.Client
  if secure {
    client = &http.Client{}
  } else {
    tr := &http.Transport{
       TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    }
    client = &http.Client{Transport: tr}
  }
  if resp, err := client.Do(rqst); err != nil {
    status = -1
    say.L1("makequery: Something wrong with network configuration or registry state: ", err, "\n")
  } else {
    defer resp.Body.Close()
    status = resp.StatusCode
    header = resp.Header
    if status == 200 || status == 401 {
      if bodytmp, err := ioutil.ReadAll(resp.Body); err != nil {
        status = -1
        say.L1("makequery: Cannot read response body: ", err, "\n")
      } else {
        var c interface{}
        if len(bodytmp) == 0 {
          body = map[string]string{}
        } else {
          if err := json.Unmarshal(bodytmp, &c); err != nil {
            status = -1
            say.L1("makequery: Cannot unmarshal response: ", err, "\n")
          } else {
            body = c
            if c.(map[string]interface{})["errors"] != nil  && status != 401 {
              say.L1("makequery: Recieved error in response: ",
                c.
                (map[string]interface{})["errors"].
                ([]interface{})[0].
                (map[string]interface{})["message"].
                (string),
              "\n")
            }
          }
        }
      }
    }
  }
  return
}

func getbearertoken(wwwauth string, user string, pass string, secure bool) (token string, ok bool){
  ok = false
  query := ""
  splitted := strings.Split(wwwauth, ",")
  if len(splitted) < 3 {
    say.L1("getbearertoken: Wrong size of obtained wwwauth: ", splitted, "\n")
  } else {
    idaddrs := strings.Index(splitted[0], "\"")
    idservc := strings.Index(splitted[1], "\"")
    idscope := strings.Index(splitted[2], "\"")
    if (idaddrs == -1) || (idservc == -1) || (idscope == -1) {
      say.L1("getbearertoken: One of the essential variables has not been recieved.", "","\n")
      say.L1("getbearertoken: Address: ", splitted[0] ,"\n")
      say.L1("getbearertoken: Service: ", splitted[1] ,"\n")
      say.L1("getbearertoken: Scope  : ", splitted[2] ,"\n")
    } else {
      if  (splitted[0][:idaddrs-1]=="Bearer realm") &&
          (splitted[1][:idservc-1]=="service") &&
          (splitted[2][:idscope-1]=="scope") {

        addrs := splitted[0][idaddrs+1:len(splitted[0])-1]
        servc := splitted[1][idservc+1:len(splitted[1])-1]
        servc = strings.Replace(servc, " ", "+", -1)
        scope := splitted[2][idscope+1:len(splitted[2])-1]
        query = addrs + "?account=" + user + "&service=" + servc + "&scope=" + scope
        if reqst, err := http.NewRequest("GET", query, nil); err != nil {
          say.L1("getbearertoken: Cannot create query: ", err, "\n")
        } else {
          reqst.Header.Add("Authorization", "Basic " +
                            base64.StdEncoding.EncodeToString([]byte(user + ":" + pass)))
          body, _, c := makequery(reqst, secure)
          if c == 200 {
            token = body.(map[string]interface{})["token"].(string)
            ok = true
          }
        }
      } else {
        say.L1("getbearertoken: Registry sent wrong Www-Authenticate header: ", wwwauth, "\n")
      }
    }
  }
  return
}

func MakeQuery(query, method string, info, inhdrs map[string]string) (body interface{}, outhdrs map[string][]string, ok bool){
  ok = false
  var c int
  secure := true
  if info["secure"] == "false" { secure = false}
  tquery := info["scheme"] + "://" + info["host"] + query
  if reqst, err := http.NewRequest(method, tquery, nil); err != nil {
    say.L1("MakeQuery: cannot create query. ", err, "\n")
  } else {
    reqst.Header.Add("Authorization", "Basic " +
                      base64.StdEncoding.EncodeToString([]byte(info["user"] + ":" + info["pass"])))
    for kh, vh := range inhdrs{
      reqst.Header.Set(kh, vh)
    }
    body, outhdrs, c = makequery(reqst, secure)
    if c == 401 {
      if outhdrs["Www-Authenticate"][0][0:5] == "Basic" {
        say.L1("MakeQuery: Code [401]: Unauthorized response is returned (credentials problem, check user/pass pair)", "","\n")
        return
      } else if outhdrs["Www-Authenticate"][0][0:6] == "Bearer" {
        say.L3("MakeQuery: Code [401]: Bearer auth. Trying to get auth token.", "", "\n")
        if token, oktok := getbearertoken(outhdrs["Www-Authenticate"][0], info["user"], info["pass"], secure); !oktok {
          say.L1("MakeQuery: Bearer: Cannot obtain token for [" + outhdrs["Www-Authenticate"][0] + "]", "","\n")
          return
        } else {
          say.L3("MakeQuery: Bearer: Token recieved. Retriying query.", "","\n")
          tquery = info["scheme"] + "://" + info["host"] + query
          if reqst, err := http.NewRequest(method, tquery, nil); err != nil {
            say.L1("MakeQuery: Cannot create request: ", err, "\n")
            return
          } else {
            reqst.Header.Add("Authorization", "Bearer "+token)
            for kh, vh := range inhdrs{
              reqst.Header.Set(kh, vh)
            }
            body, outhdrs, c = makequery(reqst, secure)
            if c == 401 {
              say.L1(
                "MakeQuery: Token: Code [401]: Unauthorized response is returned (credentials problem, check user/pass pair or communication between registry and auth server)",
                body.(map[string]interface{})["errors"].([]interface{})[0].(map[string]interface{})["message"].(string),
                "\n")
              return
            }
          }
        }
      }
    }
    if c != 200 { body = c }
    switch c {
    case 200:
      if method=="GET" || method=="HEAD" { ok = true } else { say.L1("MakeQuery: Unexpected [200] status", "","\n")}
    case 202:
      if method=="DELETE"{ ok = true } else { say.L1("MakeQuery: Unexpected [202] status", "","\n")}
    case 404:
      say.L1("MakeQuery: [404] Page not found.", "","\n")
    case -1:
      say.L1("MakeQuery: Netwrok or internal problem.", "","\n")
    default:
      say.L1("MakeQuery: Cannot diagnose problem. Code [ ",strconv.Itoa(c), " ].\n")
    }
  }
  return
}
