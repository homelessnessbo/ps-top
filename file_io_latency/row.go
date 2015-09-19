// Package file_io_latency contains the routines for
// managing the file_summary_by_instance table.
package file_io_latency

import (
	"fmt"
	"regexp"

	"github.com/sjmudd/ps-top/key_value_cache"
	"github.com/sjmudd/ps-top/lib"
	"github.com/sjmudd/ps-top/logger"
	"github.com/sjmudd/ps-top/rc"
)

/*
CREATE TABLE `file_summary_by_instance` (
  `FILE_NAME` varchar(512) NOT NULL,
  `EVENT_NAME` varchar(128) NOT NULL,				// not collected
  `OBJECT_INSTANCE_BEGIN` bigint(20) unsigned NOT NULL,		// not collected
  `COUNT_STAR` bigint(20) unsigned NOT NULL,
  `SUM_TIMER_WAIT` bigint(20) unsigned NOT NULL,
  `MIN_TIMER_WAIT` bigint(20) unsigned NOT NULL,
  `AVG_TIMER_WAIT` bigint(20) unsigned NOT NULL,
  `MAX_TIMER_WAIT` bigint(20) unsigned NOT NULL,
  `COUNT_READ` bigint(20) unsigned NOT NULL,
  `SUM_TIMER_READ` bigint(20) unsigned NOT NULL,
  `MIN_TIMER_READ` bigint(20) unsigned NOT NULL,
  `AVG_TIMER_READ` bigint(20) unsigned NOT NULL,
  `MAX_TIMER_READ` bigint(20) unsigned NOT NULL,
  `SUM_NUMBER_OF_BYTES_READ` bigint(20) NOT NULL,
  `COUNT_WRITE` bigint(20) unsigned NOT NULL,
  `SUM_TIMER_WRITE` bigint(20) unsigned NOT NULL,
  `MIN_TIMER_WRITE` bigint(20) unsigned NOT NULL,
  `AVG_TIMER_WRITE` bigint(20) unsigned NOT NULL,
  `MAX_TIMER_WRITE` bigint(20) unsigned NOT NULL,
  `SUM_NUMBER_OF_BYTES_WRITE` bigint(20) NOT NULL,
  `COUNT_MISC` bigint(20) unsigned NOT NULL,
  `SUM_TIMER_MISC` bigint(20) unsigned NOT NULL,
  `MIN_TIMER_MISC` bigint(20) unsigned NOT NULL,
  `AVG_TIMER_MISC` bigint(20) unsigned NOT NULL,
  `MAX_TIMER_MISC` bigint(20) unsigned NOT NULL
) ENGINE=PERFORMANCE_SCHEMA DEFAULT CHARSET=utf8
1 row in set (0.00 sec)
*/

// Row contains a row from file_summary_by_instance
type Row struct {
	name                  string
	countStar             uint64
	countRead             uint64
	countWrite            uint64
	countMisc             uint64
	sumTimerWait          uint64
	sumTimerRead          uint64
	sumTimerWrite         uint64
	sumTimerMisc          uint64
	sumNumberOfBytesRead  uint64
	sumNumberOfBytesWrite uint64
}

//     foo/../bar --> foo/bar   perl: $new =~ s{[^/]+/\.\./}{/};
//     /./        --> /         perl: $new =~ s{/\./}{};
//     //         --> /         perl: $new =~ s{//}{/};
const (
	reEncoded = `@(\d{4})` // FIXME - add me to catch @0024 --> $ for example
)

var (
	reOneOrTheOther    = regexp.MustCompile(`/(\.)?/`)
	reSlashDotDotSlash = regexp.MustCompile(`[^/]+/\.\./`)
	reTableFile        = regexp.MustCompile(`/([^/]+)/([^/]+)\.(frm|ibd|MYD|MYI|CSM|CSV|par)$`)
	reTempTable        = regexp.MustCompile(`#sql-[0-9_]+`)
	rePartTable        = regexp.MustCompile(`(.+)#P#p(\d+|MAX)`)
	reIbdata           = regexp.MustCompile(`/ibdata\d+$`)
	reIbtmp            = regexp.MustCompile(`/ibtmp\d+$`)
	reRedoLog          = regexp.MustCompile(`/ib_logfile\d+$`)
	reBinlog           = regexp.MustCompile(`/binlog\.(\d{6}|index)$`)
	reDbOpt            = regexp.MustCompile(`/db\.opt$`)
	reSlowlog          = regexp.MustCompile(`/slowlog$`)
	reAutoCnf          = regexp.MustCompile(`/auto\.cnf$`)
	rePidFile          = regexp.MustCompile(`/[^/]+\.pid$`)
	reErrorMsg         = regexp.MustCompile(`/share/[^/]+/errmsg\.sys$`)
	reCharset          = regexp.MustCompile(`/share/charsets/Index\.xml$`)
	reDollar           = regexp.MustCompile(`@0024`) // FIXME - add me to catch @0024 --> $ (specific case)

	cache key_value_cache.KeyValueCache
)

func (row Row) headings() string {
	return fmt.Sprintf("%10s %6s|%6s %6s %6s|%8s %8s|%8s %6s %6s %6s|%s",
		"Latency",
		"%",
		"Read",
		"Write",
		"Misc",
		"Rd bytes",
		"Wr bytes",
		"Ops",
		"R Ops",
		"W Ops",
		"M Ops",
		"Table Name")
}

func (row Row) String() string {
	return fmt.Sprintf("%s: %9d %9d %9d %9d %9d %9d %9d %9d %9d %9d",
		row.name,
		row.countStar,
		row.countRead,
		row.countWrite,
		row.countMisc,
		row.sumTimerWait,
		row.sumTimerRead,
		row.sumTimerWrite,
		row.sumTimerMisc,
		row.sumNumberOfBytesRead,
		row.sumNumberOfBytesWrite)
}

// validate if any of the values seem out of bounds
func (row Row) validate() {
	if (row.countStar < row.countRead) ||
		(row.countStar < row.countWrite) ||
		(row.countStar < row.countMisc) {
		logger.Println("Row.validate() FAILED (count)", row)
	}
	if (row.sumTimerWait < row.sumTimerRead) ||
		(row.sumTimerWait < row.sumTimerWrite) ||
		(row.sumTimerWait < row.sumTimerMisc) {
		logger.Println("Row.validate() FAILED (timer)", row)
	}
}

// generate a printable result
func (row Row) rowContent(totals Row) string {
	var name = row.name

	row.validate()

	// We assume that if countStar = 0 then there's no data at all...
	// when we have no data we really don't want to show the name either.
	if row.sumTimerWait == 0 && name != "Totals" {
		name = ""
	}

	return fmt.Sprintf("%10s %6s|%6s %6s %6s|%8s %8s|%8s %6s %6s %6s|%s",
		lib.FormatTime(row.sumTimerWait),
		lib.FormatPct(lib.MyDivide(row.sumTimerWait, totals.sumTimerWait)),
		lib.FormatPct(lib.MyDivide(row.sumTimerRead, row.sumTimerWait)),
		lib.FormatPct(lib.MyDivide(row.sumTimerWrite, row.sumTimerWait)),
		lib.FormatPct(lib.MyDivide(row.sumTimerMisc, row.sumTimerWait)),
		lib.FormatAmount(row.sumNumberOfBytesRead),
		lib.FormatAmount(row.sumNumberOfBytesWrite),
		lib.FormatAmount(row.countStar),
		lib.FormatPct(lib.MyDivide(row.countRead, row.countStar)),
		lib.FormatPct(lib.MyDivide(row.countWrite, row.countStar)),
		lib.FormatPct(lib.MyDivide(row.countMisc, row.countStar)),
		name)
}

func (row *Row) add(other Row) {
	row.countStar += other.countStar
	row.countRead += other.countRead
	row.countWrite += other.countWrite
	row.countMisc += other.countMisc

	row.sumTimerWait += other.sumTimerWait
	row.sumTimerRead += other.sumTimerRead
	row.sumTimerWrite += other.sumTimerWrite
	row.sumTimerMisc += other.sumTimerMisc

	row.sumNumberOfBytesRead += other.sumNumberOfBytesRead
	row.sumNumberOfBytesWrite += other.sumNumberOfBytesWrite
}

// subtract one set of values from another one.
func (row *Row) subtract(other Row) bool {
	var bug bool

	if row.sumTimerWait < other.sumTimerWait {
		logger.Println("BUG: file_io_latency subtraction problem:")
		logger.Println("row:  ", *row)
		logger.Println("other:", other)
		bug = true

		// set the subtraction to zero
		row.countStar = 0
		row.countRead = 0
		row.countWrite = 0
		row.countMisc = 0

		row.sumTimerWait = 0
		row.sumTimerRead = 0
		row.sumTimerWrite = 0
		row.sumTimerMisc = 0

		row.sumNumberOfBytesRead = 0
		row.sumNumberOfBytesWrite = 0
	} else {
		row.countStar -= other.countStar
		row.countRead -= other.countRead
		row.countWrite -= other.countWrite
		row.countMisc -= other.countMisc

		row.sumTimerWait -= other.sumTimerWait
		row.sumTimerRead -= other.sumTimerRead
		row.sumTimerWrite -= other.sumTimerWrite
		row.sumTimerMisc -= other.sumTimerMisc

		row.sumNumberOfBytesRead -= other.sumNumberOfBytesRead
		row.sumNumberOfBytesWrite -= other.sumNumberOfBytesWrite
	}

	return bug
}

// From the original name we want to generate a simpler name to use.
// This simpler name may also merge several different filenames into one.
func (row Row) simplifyName(globalVariables map[string]string) string {
	path := row.name

	if cachedResult, err := cache.Get(path); err == nil {
		return cachedResult
	}

	// @0024 --> $ (should do this more generically)
	path = reDollar.ReplaceAllLiteralString(path, "$")

	// this should probably be ordered from most expected regexp to least
	if m1 := reTableFile.FindStringSubmatch(path); m1 != nil {
		// we may match temporary tables so check for them
		if m2 := reTempTable.FindStringSubmatch(m1[2]); m2 != nil {
			return cache.Put(path, "<temp_table>")
		}

		// we may match partitioned tables so check for them
		if m3 := rePartTable.FindStringSubmatch(m1[2]); m3 != nil {
			return cache.Put(path, lib.TableName(m1[1], m3[1])) // <schema>.<table> (less partition info)
		}

		return cache.Put(path, rc.Munge(lib.TableName(m1[1], m1[2]))) // <schema>.<table>
	}
	if reIbtmp.MatchString(path) {
		return cache.Put(path, "<ibtmp>")
	}
	if reIbdata.MatchString(path) {
		return cache.Put(path, "<ibdata>")
	}
	if reRedoLog.MatchString(path) {
		return cache.Put(path, "<redo_log>")
	}
	if reBinlog.MatchString(path) {
		return cache.Put(path, "<binlog>")
	}
	if reDbOpt.MatchString(path) {
		return cache.Put(path, "<db_opt>")
	}
	if reSlowlog.MatchString(path) {
		return cache.Put(path, "<slow_log>")
	}
	if reAutoCnf.MatchString(path) {
		return cache.Put(path, "<auto_cnf>")
	}
	// relay logs are a bit complicated. If a full path then easy to
	// identify, but if a relative path we may need to add $datadir,
	// but also if as I do we have a ../blah/somewhere/path then we
	// need to make it match too.
	if len(globalVariables["relay_log"]) > 0 {
		relayLog := globalVariables["relay_log"]
		if relayLog[0] != '/' { // relative path
			relayLog = cleanupPath(globalVariables["datadir"] + relayLog) // datadir always ends in /
		}
		reRelayLog := relayLog + `\.(\d{6}|index)$`
		if regexp.MustCompile(reRelayLog).MatchString(path) {
			return cache.Put(path, "<relay_log>")
		}
	}
	if rePidFile.MatchString(path) {
		return cache.Put(path, "<pid_file>")
	}
	if reErrorMsg.MatchString(path) {
		return cache.Put(path, "<errmsg>")
	}
	if reCharset.MatchString(path) {
		return cache.Put(path, "<charset>")
	}
	// clean up datadir to <datadir>
	if len(globalVariables["datadir"]) > 0 {
		reDatadir := regexp.MustCompile("^" + globalVariables["datadir"])
		path = reDatadir.ReplaceAllLiteralString(path, "<datadir>/")
	}

	return cache.Put(path, path)
}

// clean up the given path reducing redundant stuff and return the clean path
func cleanupPath(path string) string {
	for {
		origPath := path
		path = reOneOrTheOther.ReplaceAllString(path, "/")
		path = reSlashDotDotSlash.ReplaceAllString(path, "/")
		if origPath == path { // no change so give up
			break
		}
	}

	return path
}