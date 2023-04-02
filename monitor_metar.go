package main

import (

	//	"math"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	//	"strings"
)

type spekStasiun struct {
	Icao    string
	Stasiun string
	Lat     string
	Lon     string
	Alt     string
}

type statStasiun struct {
	Icao        string
	Nama        string
	WaktuOn     []time.Time
	WaktuInsert []time.Time
}

type metarRaw struct {
	Centre_code  string
	Data_text    string
	Insert_time  time.Time
	Filling_time time.Time
}

func getAnticipatedTime(t time.Time) time.Time {
	minute := t.Minute()
	minuteAnticipated := 30
	if minute < 10 {
		minuteAnticipated = 0
		return t.Add(-time.Duration(minute) * time.Minute).Add(-time.Duration(t.Second()) * time.Second).Add(time.Duration(minuteAnticipated) * time.Minute)
	} else if minute > 50 {
		minuteAnticipated = 0
		return t.Add(-time.Duration(minute) * time.Minute).Add(-time.Duration(t.Second()) * time.Second).Add(time.Duration(60) * time.Minute)
	}
	return t.Add(-time.Duration(minute) * time.Minute).Add(-time.Duration(t.Second()) * time.Second).Add(time.Duration(minuteAnticipated) * time.Minute)
}

//bson.M adalah alias dari map["string"] interface{}

func setupRouter() *gin.Engine {
	r := gin.Default()
	store := cookie.NewStore([]byte("monitor_metar"))
	store.Options(sessions.Options{
		MaxAge: 60 * 60,
	})

	r.Static("/static_monitor_metar", "./static")
	r.LoadHTMLGlob("templates/*")

	//get slice stasiun
	jsonData, err := ioutil.ReadFile("list_stasiun.json")
	if err != nil {
		log.Fatal(err)
	}

	// Unmarshal the JSON data into a slice of Person structs
	var all_stasiun []spekStasiun
	err = json.Unmarshal(jsonData, &all_stasiun)
	if err != nil {
		log.Fatal(err)
	}

	mysql_str := fmt.Sprintf("genomexyz:reyditya@tcp(%s)/%s?parseTime=true", "127.0.0.1", "raw_data")
	//mysql_str := fmt.Sprintf("netadmin:r00tBMKG@!)&@tcp(%s)/%s?parseTime=true", "172.19.2.98", "raw_data")
	db, err := sql.Open("mysql", mysql_str)
	// if there is an error opening the connection, handle it
	if err != nil {
		panic(err.Error())
	}
	fmt.Println(db)
	fmt.Println(all_stasiun)

	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	r.GET("/index", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	r.GET("/get_data_date_detail/:date", func(c *gin.Context) {
		tanggal := c.Param("date")
		val := bson.M{}
		tanggal_time, err := time.Parse(time.RFC3339, tanggal)
		if err != nil {
			fmt.Println(err)
			val["status"] = "err"
			c.JSON(200, val)
			return
		}
		tanggal_time_anticipated := getAnticipatedTime(tanggal_time)
		fmt.Println("cek anticipated time", tanggal_time_anticipated)

		all_statstasiun := make(map[string]*statStasiun)
		check_stasiun := make(map[string]bool)
		for iter_stasiun := range all_stasiun {
			single_stasiun_icao := all_stasiun[iter_stasiun].Icao
			single_stasiun_nama := all_stasiun[iter_stasiun].Stasiun
			var single_statstasiun statStasiun
			single_statstasiun.Icao = single_stasiun_icao
			single_statstasiun.Nama = single_stasiun_nama
			single_statstasiun.WaktuOn = make([]time.Time, 0)
			single_statstasiun.WaktuInsert = make([]time.Time, 0)
			check_stasiun[single_stasiun_icao] = true
			all_statstasiun[single_stasiun_icao] = &single_statstasiun
		}
		val["stasiun"] = all_statstasiun

		stmt, err := db.Prepare("select centre_code, data_text, insert_time, filling_time from metar_speci where filling_time = ? and type_code = ?")
		if err != nil {
			fmt.Println(err)
			val["status"] = "err"
			c.JSON(200, val)
			return
		}
		defer stmt.Close()

		rows, err := stmt.Query(tanggal_time_anticipated, "SA")
		if err != nil {
			fmt.Println(err)
			val["status"] = "err"
			c.JSON(200, val)
			return
		}
		defer rows.Close()

		// Iterate over the results
		for rows.Next() {
			var centre_code, data_text string
			var fill_time, insert_time time.Time
			err := rows.Scan(&centre_code, &data_text, &insert_time, &fill_time)
			if err != nil {
				fmt.Println(err)
				val["status"] = "err"
				c.JSON(200, val)
				return
			}
			if !check_stasiun[centre_code] {
				continue
			}
			val["stasiun"].(map[string]*statStasiun)[centre_code].WaktuOn = append(val["stasiun"].(map[string]*statStasiun)[centre_code].WaktuOn, fill_time)
			val["stasiun"].(map[string]*statStasiun)[centre_code].WaktuInsert = append(val["stasiun"].(map[string]*statStasiun)[centre_code].WaktuInsert, insert_time)
			fmt.Println(centre_code, data_text, insert_time, fill_time)
		}
		fmt.Println(val)
		err = rows.Err()
		if err != nil {
			fmt.Println(err)
			val["status"] = "err"
			c.JSON(200, val)
			return
		}

		val["status"] = "ok"
		c.JSON(200, val)
		return
	})

	r.GET("/get_data_date/:date", func(c *gin.Context) {
		tanggal := c.Param("date")
		val := bson.M{}
		tanggal_awal := tanggal + "T00:00:00Z"
		tanggal_akhir := tanggal + "T23:59:00Z"

		tanggal_awal_time, err := time.Parse(time.RFC3339, tanggal_awal)
		if err != nil {
			fmt.Println(err)
			val["status"] = "err"
			c.JSON(200, val)
			return
		}

		tanggal_akhir_time, err := time.Parse(time.RFC3339, tanggal_akhir)
		if err != nil {
			fmt.Println(err)
			val["status"] = "err"
			c.JSON(200, val)
			return
		}

		//get all
		//val["stasiun"] = make([]statStasiun, 0)
		all_statstasiun := make(map[string]*statStasiun)
		check_stasiun := make(map[string]bool)
		for iter_stasiun := range all_stasiun {
			single_stasiun_icao := all_stasiun[iter_stasiun].Icao
			single_stasiun_nama := all_stasiun[iter_stasiun].Stasiun
			var single_statstasiun statStasiun
			single_statstasiun.Icao = single_stasiun_icao
			single_statstasiun.Nama = single_stasiun_nama
			single_statstasiun.WaktuOn = make([]time.Time, 0)
			check_stasiun[single_stasiun_icao] = true
			all_statstasiun[single_stasiun_icao] = &single_statstasiun
		}
		val["stasiun"] = all_statstasiun

		stmt, err := db.Prepare("select centre_code, data_text, insert_time, filling_time from metar_speci where filling_time >= ? and filling_time < ? and type_code = ?")
		if err != nil {
			fmt.Println(err)
			val["status"] = "err"
			c.JSON(200, val)
			return
		}
		defer stmt.Close()

		rows, err := stmt.Query(tanggal_awal_time, tanggal_akhir_time, "SA")
		if err != nil {
			fmt.Println(err)
			val["status"] = "err"
			c.JSON(200, val)
			return
		}
		defer rows.Close()

		// Iterate over the results
		for rows.Next() {
			var centre_code, data_text string
			var fill_time, insert_time time.Time
			err := rows.Scan(&centre_code, &data_text, &insert_time, &fill_time)
			if err != nil {
				fmt.Println(err)
				val["status"] = "err"
				c.JSON(200, val)
				return
			}
			if !check_stasiun[centre_code] {
				continue
			}
			val["stasiun"].(map[string]*statStasiun)[centre_code].WaktuOn = append(val["stasiun"].(map[string]*statStasiun)[centre_code].WaktuOn, fill_time)
			val["stasiun"].(map[string]*statStasiun)[centre_code].WaktuInsert = append(val["stasiun"].(map[string]*statStasiun)[centre_code].WaktuInsert, insert_time)
			fmt.Println(centre_code, data_text, insert_time, fill_time)
		}
		fmt.Println(val)
		err = rows.Err()
		if err != nil {
			fmt.Println(err)
			val["status"] = "err"
			c.JSON(200, val)
			return
		}

		val["status"] = "ok"
		c.JSON(200, val)
	})

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/monitor_metar/index")
	})

	return r
}

func main() {
	r := setupRouter()
	r.Run(":7654")
}
