package store

// Registra o driver MySQL para o sql.Open("mysql", ...) de Open.
//
// Fica no proprio pacote, e nao em cmd/, para que Open funcione sem o
// chamador ter de lembrar do import em branco.
import _ "github.com/go-sql-driver/mysql"
