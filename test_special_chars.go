package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// 连接到代理服务器
	db, err := sql.Open("mysql", "root:eQrKBKAQmQShzG6c@tcp(localhost:3310)/mysql")
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	// 测试1: 包含换行符的查询
	query1 := "SELECT VERSION();\nSELECT NOW();"
	rows, err := db.Query(query1)
	if err != nil {
		fmt.Printf("执行查询1失败: %v\n", err)
	} else {
		rows.Close()
		fmt.Println("查询1执行成功 (包含换行符)")
	}

	// 测试2: 包含控制字符的查询
	query2 := "SELECT 'Value with control chars: ' || CHAR(10) || CHAR(13) || CHAR(9) || 'end';"
	rows, err = db.Query(query2)
	if err != nil {
		fmt.Printf("执行查询2失败: %v\n", err)
	} else {
		rows.Close()
		fmt.Println("查询2执行成功 (包含控制字符)")
	}

	// 测试3: 包含Unicode字符的查询
	query3 := "SELECT 'Unicode测试: \u03B1\u03B2\u03B3';"
	rows, err = db.Query(query3)
	if err != nil {
		fmt.Printf("执行查询3失败: %v\n", err)
	} else {
		rows.Close()
		fmt.Println("查询3执行成功 (包含Unicode字符)")
	}

	fmt.Println("所有测试完成，请查看faraday日志以验证特殊字符处理效果")
}