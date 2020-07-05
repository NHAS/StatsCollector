package webservice

import (
	"encoding/hex"
	"fmt"
	"html/template"
	"time"

	"github.com/NHAS/StatsCollector/models"
	"github.com/gorilla/csrf"
)

func humanDate(epoch int64) string {
	t1 := time.Unix(epoch, 0)
	epochYear, epochMonth, epochDay := t1.Date()

	epochHour := t1.Hour()
	epochMin := t1.Minute()

	t2 := time.Now()
	currentYear, currentMonth, currentDay := t2.Date()

	if currentYear == epochYear && currentMonth == epochMonth && currentDay == epochDay {
		return fmt.Sprintf("Today at %02d:%02d", epochHour, epochMin)
	}

	if currentYear == epochYear {
		return fmt.Sprintf("%02d:%02d %s %d", epochHour, epochMin, epochMonth.String()[:3], epochDay)
	}

	return fmt.Sprintf("%02d:%02d %s %d %d", epochHour, epochMin, epochMonth.String()[:3], epochDay, epochYear)
}

func humanTime(time time.Time) string {
	return humanDate(time.Unix())
}

func limitPrint(number float32) string {
	return fmt.Sprintf("%.2f", number)
}

func hexEncode(s string) string {
	return hex.EncodeToString([]byte(s))
}

func wrap(agent models.Agent, csrfElement template.HTML) map[string]interface{} {

	return map[string]interface{}{
		"Agent":          agent,
		csrf.TemplateTag: csrfElement,
	}
}
