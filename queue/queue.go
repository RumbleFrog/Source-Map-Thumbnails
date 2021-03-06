package queue

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/rumblefrog/Source-Map-Thumbnails/meta"
	"github.com/rumblefrog/Source-Map-Thumbnails/postprocessor"
	"github.com/rumblefrog/Source-Map-Thumbnails/preprocessor"
	"github.com/rumblefrog/Source-Map-Thumbnails/rcon"
	"github.com/rumblefrog/Source-Map-Thumbnails/utils"
	"github.com/sirupsen/logrus"

	"github.com/rumblefrog/Source-Map-Thumbnails/spawner"
)

type Queue_t struct {
	Position      int
	Maps          []string
	PreProcessor  *preprocessor.PreProcessor_t
	PostProcessor *postprocessor.PostProcessor_t
	Wait          chan int8
	Connection    *rcon.RemoteConsole
}

func NewQueue() (q *Queue_t) {
	q = &Queue_t{
		Position:      0,
		Maps:          nil, // We will initialize this once we read the directory
		PreProcessor:  preprocessor.NewPreProcessor(),
		PostProcessor: postprocessor.NewPostProcessor(),
		Wait:          make(chan int8, 1),
	}

	return
}

func (q *Queue_t) Start() {
	q.Populate()

	logrus.Infof("Populated %d maps", len(q.Maps))

	time.AfterFunc(40*time.Second, q.AttemptConnect)

	<-q.Wait

	q.ProcessItem()
}

func (q *Queue_t) ProcessItem() {
	// Match status map to see if we need to change map
	q.ChangeLevel()

	<-q.Wait

	q.Setup()

	<-q.Wait

	logrus.Debugf("Map ready")

	q.Screenshot()

	<-q.Wait

	q.Next()
}

func (q *Queue_t) Screenshot() {
	nodes := q.GetNodes()

	if nodes == nil {
		q.Screenshot()

		return
	}

	var query strings.Builder

	query.WriteString("wait 66;")

	for i := 0; i < len(nodes); i++ {
		query.WriteString("jpeg;wait 66;spec_next;wait 66;")
	}

	err := q.Connection.WriteNoReply(query.String())

	if err != nil {
		q.ScreenshotTimed()

		return
	}

	time.Sleep(time.Duration(850*(len(nodes)+1)) * time.Millisecond)

	logrus.WithFields(logrus.Fields{
		"Map":       q.Maps[q.Position],
		"NodeCount": len(nodes),
		"Remaining": len(q.Maps) - q.Position - 1,
	}).Info("Map processed")

	q.PostProcessor.Run(meta.Map_t{
		Name:      q.Maps[q.Position],
		Count:     len(nodes),
		Positions: nodes,
	})

	q.Wait <- 1
}

func (q *Queue_t) GetNodes() []meta.Position_t {
	// Forced to re-allocate as we iterate each node
	nodes := make([]meta.Position_t, 0)

	process := true

	for process == true {
		reqID, err := q.Connection.Write("spec_pos;spec_next")

		if err != nil {
			process = false

			return nil
		}

		res, resID, err := q.Connection.Read()

		resPos := utils.ParseSpecPos(res)

		if err != nil || resID != reqID {
			process = false

			return nil
		}

		for _, v := range nodes {
			if v.IsEqual(resPos) {
				process = false

				return nodes
			}
		}

		nodes = append(nodes, resPos)

		// Possible rotating spectator point (never ending)
		if len(nodes) > 50 {
			logrus.WithField("Map", q.Maps[q.Position]).Warn("Skipping due to possible infinitely rotating node")

			q.Next()
		}

		time.Sleep(200 * time.Millisecond)
	}

	return nil
}

func (q *Queue_t) ChangeLevel() {
	err := q.Connection.WriteNoReply("changelevel " + q.Maps[q.Position])

	if err != nil {
		time.AfterFunc(5*time.Second, q.ChangeLevel)

		return
	}

	logrus.WithField("NextMap", q.Maps[q.Position]).Info("Changing level")

	time.AfterFunc(10*time.Second, q.CheckMap)
}

func (q *Queue_t) CheckMap() {
	_, err := q.Connection.Write("status")

	if err != nil {
		q.CheckMapTimed()

		return
	}

	res, _, err := q.Connection.Read()

	if err != nil {
		q.CheckMapTimed()

		logrus.Debug("Unable to read status. Retrying in 5s.")

		return
	}

	mapMatches := utils.MapRegex.FindStringSubmatch(res)
	cStateMatches := utils.CStateRegex.FindStringSubmatch(res)

	if len(mapMatches) < 2 || len(cStateMatches) < 8 {
		q.CheckMapTimed()

		logrus.Debug("Map data short. Retrying in 5s.")

		return
	}

	if mapMatches[1] != q.Maps[q.Position] || cStateMatches[7] != "active" {
		q.CheckMapTimed()

		logrus.WithFields(logrus.Fields{
			"MapCheck":   q.Maps[q.Position] + " == " + mapMatches[1],
			"StateCheck": "active == " + cStateMatches[7],
		}).Debug("Map data mismatch. Retrying in 5s.")

		return
	}

	q.Wait <- 1
}

func (q *Queue_t) Setup() {
	err := q.Connection.WriteNoReply("cl_drawhud 0;spec_mode;jpeg_quality 100;sv_cheats 1;sv_cheats 0;")

	if err != nil {
		q.SetupTimed()

		return
	}

	q.Wait <- 1
}

func (q *Queue_t) CheckMapTimed() {
	time.AfterFunc(5*time.Second, q.CheckMap)
}

func (q *Queue_t) ScreenshotTimed() {
	time.AfterFunc(5*time.Second, q.Screenshot)
}

func (q *Queue_t) SetupTimed() {
	time.AfterFunc(5*time.Second, q.SetupTimed)
}

func (q *Queue_t) More() bool {
	return len(q.Maps) > q.Position+1
}

func (q *Queue_t) Next() {
	if !q.More() {
		logrus.WithField("Count", len(q.Maps)).Info("Finished processing maps")

		q.Terminate()

		return
	}

	q.Position++

	q.ProcessItem()
}

func (q *Queue_t) AttemptConnect() {
	var err error

	q.Connection, err = rcon.Dial(utils.GetFirstLocalIPv4()+":27015", "smt")

	if err != nil || q.Connection == nil {
		time.AfterFunc(10*time.Second, q.AttemptConnect)

		logrus.Debug("RCON connection failed. Retrying in 10s")

		return
	}

	logrus.Info("RCON connection established")

	q.Wait <- 1
}

// Calling this will also stop the block at .Wait, causing it to send an int to main to finish cleaning up
func (q *Queue_t) Terminate() error {
	if q.Connection != nil {
		q.Connection.Close()
	}

	return spawner.Command.Process.Kill()
}

func (q *Queue_t) Populate() {
	files, err := ioutil.ReadDir(utils.GamePathJoin("maps"))

	q.Maps = make([]string, 0, len(files)) // Let's pass a capacity here to prevent slice reallocation (slightly bigger is fine)

	if err != nil {
		q.Terminate()
		return
	}

	var mName string

	for _, file := range files {
		if !file.IsDir() {
			mName = filepath.Base(file.Name())
			mName = strings.TrimSuffix(mName, filepath.Ext(mName))

			if !q.PreProcessor.Run(mName) {
				continue
			}

			q.Maps = append(q.Maps, mName)
		}
	}
}
