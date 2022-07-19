package database

import (
	"fmt"
	"log"
)

func (db *DB) AddSpaceUsageReport(unixtime int64, bcount int64) error {
	// Insert the new entry
	stmtIns, err := db.dbConn.Prepare("INSERT INTO space_usage_history (date, space_used) VALUES (?, ?)")
	if err != nil {
		log.Println("error: AddSpaceUsageReport: ", err)
		return err
	}
	defer stmtIns.Close()
	_, err = stmtIns.Exec(unixtime, bcount)
	if err != nil {
		log.Println("error: AddSpaceUsageReport: ", err)
		return err
	}

	// Delete anything older than last 3 months to keep table tidy
	stmtDel, err := db.dbConn.Prepare("DELETE FROM space_usage_history WHERE DATE(date,'unixepoch','localtime') < DATE('now','localtime','-3 month','start of month')")
	if err != nil {
		log.Println("error: AddSpaceUsageReport: ", err)
		return err
	}
	defer stmtDel.Close()
	_, err = stmtDel.Exec()
	if err != nil {
		log.Println("error: AddSpaceUsageReport: ", err)
		return err
	}

	return nil
}

func (db *DB) AddBandwidthUsageReport(unixtime int64, bcount int64) error {
	// Insert the new entry
	stmtIns, err := db.dbConn.Prepare("INSERT INTO bandwidth_usage_history (date, bandwidth_used) VALUES (?, ?)")
	if err != nil {
		log.Println("error: AddBandwidthUsageReport: ", err)
		return err
	}
	defer stmtIns.Close()
	_, err = stmtIns.Exec(unixtime, bcount)
	if err != nil {
		log.Println("error: AddBandwidthUsageReport: ", err)
		return err
	}

	// Delete anything older than last 3 months to keep table tidy
	stmtDel, err := db.dbConn.Prepare("DELETE FROM bandwidth_usage_history WHERE DATE(date,'unixepoch','localtime') < DATE('now','localtime','-3 month','start of month')")
	if err != nil {
		log.Println("error: AddBandwidthUsageReport: ", err)
		return err
	}
	defer stmtDel.Close()
	_, err = stmtDel.Exec()
	if err != nil {
		log.Println("error: AddBandwidthUsageReport: ", err)
		return err
	}

	return nil
}

type PeakDailySpaceUsage struct {
	DateYMD string
	ByteCnt int64
}

func (db *DB) GetPeakDailySpaceUsage(prevNMonths int) ([]PeakDailySpaceUsage, error) {
	query := fmt.Sprintf("SELECT DATE(date,'unixepoch', 'localtime'), MAX(space_used) FROM space_usage_history WHERE DATE(date,'unixepoch', 'localtime') > DATE('now','localtime','-%d month','start of month') GROUP BY DATE(date,'unixepoch', 'localtime') ORDER BY date ASC;", prevNMonths)
	rows, err := db.dbConn.Query(query)
	if err != nil {
		log.Println("error: GetPeakDailySpaceUsage: ", err)
		return nil, err
	}
	defer rows.Close()
	ret := make([]PeakDailySpaceUsage, 0, prevNMonths*31)
	for rows.Next() {
		var ymd string
		var peakSpaceUsesd int64
		err = rows.Scan(&ymd, &peakSpaceUsesd)
		if err != nil {
			log.Println("error: GetPeakDailySpaceUsage: ", err)
			return nil, err
		}
		ret = append(ret, PeakDailySpaceUsage{
			DateYMD: ymd,
			ByteCnt: peakSpaceUsesd,
		})
	}
	err = rows.Err()
	if err != nil {
		log.Println("error: GetPeakDailySpaceUsage: ", err)
		return nil, err
	}

	return ret, nil
}

type TotalDailyBandwidthUsage struct {
	DateYMD string
	ByteCnt int64
}

func (db *DB) GetTotalDailyBandwidthUsage(prevNMonths int) ([]TotalDailyBandwidthUsage, error) {
	query := fmt.Sprintf("SELECT DATE(date,'unixepoch', 'localtime'), SUM(bandwidth_used) FROM bandwidth_usage_history WHERE DATE(date,'unixepoch', 'localtime') > DATE('now','localtime','-%d month','start of month') GROUP BY DATE(date,'unixepoch', 'localtime') ORDER BY date ASC;", prevNMonths)
	rows, err := db.dbConn.Query(query)
	if err != nil {
		log.Println("error: GetTotalDailyBandwidthUsage: ", err)
		return nil, err
	}
	defer rows.Close()
	ret := make([]TotalDailyBandwidthUsage, 0, prevNMonths*31)
	for rows.Next() {
		var ymd string
		var totalBandwidthUsesd int64
		err = rows.Scan(&ymd, &totalBandwidthUsesd)
		if err != nil {
			log.Println("error: GetTotalDailyBandwidthUsage: ", err)
			return nil, err
		}
		ret = append(ret, TotalDailyBandwidthUsage{
			DateYMD: ymd,
			ByteCnt: totalBandwidthUsesd,
		})
	}
	err = rows.Err()
	if err != nil {
		log.Println("error: GetTotalDailyBandwidthUsage: ", err)
		return nil, err
	}

	return ret, nil
}
