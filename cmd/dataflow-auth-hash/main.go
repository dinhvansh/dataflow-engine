package main
import (
 "bufio"
 "fmt"
 "os"
 "strings"
 "golang.org/x/crypto/bcrypt"
)
func main(){
 reader:=bufio.NewReader(os.Stdin)
 raw,e:=reader.ReadString('\n')
 if e!=nil&&len(raw)==0 {fmt.Fprintln(os.Stderr,"password required on stdin");os.Exit(1)}
 password:=strings.TrimRight(raw,"\r\n")
 if len(password)<12||len(password)>256{fmt.Fprintln(os.Stderr,"use a unique 12–256 character password");os.Exit(1)}
 hash,e:=bcrypt.GenerateFromPassword([]byte(password),bcrypt.DefaultCost)
 if e!=nil{fmt.Fprintln(os.Stderr,"hash unavailable");os.Exit(1)}
 fmt.Println(string(hash))
}
