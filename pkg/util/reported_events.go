package util

type ReportedEventKind int64

const (
	ERR_OP_NOT_PERMITTED              ReportedEventKind = 1
	INFO_BACKUP_COMPLETED             ReportedEventKind = 2
	ERR_INCOMPATIBLE_BUCKET_VERSION   ReportedEventKind = 3
	INFO_BACKUP_COMPLETED_WITH_ERRORS ReportedEventKind = 4
	INFO_BACKUP_CANCELED              ReportedEventKind = 5
	INFO_AUTOPRUNE_COMPLETED          ReportedEventKind = 6
)

type ReportedEvent struct {
	Kind     ReportedEventKind
	Path     string
	IsDir    bool
	Datetime int64
	Msg      string
}
