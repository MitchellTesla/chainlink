package controllers_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/h2non/gock"
	. "github.com/onsi/gomega"
	"github.com/smartcontractkit/chainlink-go/internal/cltest"
	"github.com/smartcontractkit/chainlink-go/models"
	"github.com/smartcontractkit/chainlink-go/models/adapters"
	"github.com/stretchr/testify/assert"
)

func TestCreateJobs(t *testing.T) {
	t.Parallel()
	store := cltest.Store()
	defer store.Close()
	server := cltest.SetUpWeb(store)
	defer cltest.TearDownWeb()

	jsonStr := cltest.LoadJSON("./fixtures/create_jobs.json")
	resp, _ := http.Post(server.URL+"/jobs", "application/json", bytes.NewBuffer(jsonStr))
	respJSON := cltest.JobJSONFromResponse(resp)
	assert.Equal(t, 200, resp.StatusCode, "Response should be success")

	var j models.Job
	store.One("ID", respJSON.ID, &j)
	sched := j.Schedule
	assert.Equal(t, j.ID, respJSON.ID, "Wrong job returned")
	assert.Equal(t, "* 7 * * *", string(sched.Cron), "Wrong cron schedule saved")
	assert.Equal(t, (*models.Time)(nil), sched.StartAt, "Wrong start at saved")
	endAt := models.Time{cltest.TimeParse("2019-11-27T23:05:49Z")}
	assert.Equal(t, endAt, *sched.EndAt, "Wrong end at saved")
	runAt0 := models.Time{cltest.TimeParse("2018-11-27T23:05:49Z")}
	assert.Equal(t, runAt0, sched.RunAt[0], "Wrong run at saved")

	adapter1, _ := j.Tasks[0].Adapter()
	httpGet := adapter1.(*adapters.HttpGet)
	assert.Equal(t, httpGet.Endpoint, "https://bitstamp.net/api/ticker/")

	adapter2, _ := j.Tasks[1].Adapter()
	jsonParse := adapter2.(*adapters.JsonParse)
	assert.Equal(t, jsonParse.Path, []string{"last"})

	adapter3, _ := j.Tasks[2].Adapter()
	bytes32 := adapter3.(*adapters.EthBytes32)
	assert.Equal(t, bytes32.Address, "0x356a04bce728ba4c62a30294a55e6a8600a320b3")
	assert.Equal(t, bytes32.FunctionID, "12345679")
}

func TestCreateJobsIntegration(t *testing.T) {
	t.Parallel()
	RegisterTestingT(t)
	defer gock.Off()

	store := cltest.Store()
	store.Start()
	defer store.Close()
	server := cltest.SetUpWeb(store)
	defer cltest.TearDownWeb()

	jsonStr := cltest.LoadJSON("./fixtures/create_hello_world_job.json")
	resp, _ := http.Post(server.URL+"/jobs", "application/json", bytes.NewBuffer(jsonStr))
	respJSON := cltest.JobJSONFromResponse(resp)

	expectedResponse := `{"high": "10744.00", "last": "10583.75", "timestamp": "1512156162", "bid": "10555.13", "vwap": "10097.98", "volume": "17861.33960013", "low": "9370.11", "ask": "10583.00", "open": "9927.29"}`
	gock.New("https://www.bitstamp.net").
		Get("/api/ticker").
		Reply(200).
		JSON(expectedResponse)

	jobRuns := []models.JobRun{}
	Eventually(func() []models.JobRun {
		_ = store.Where("JobID", respJSON.ID, &jobRuns)
		return jobRuns
	}).Should(HaveLen(1))

	store.Scheduler.Stop()

	var job models.Job
	err := store.One("ID", respJSON.ID, &job)
	assert.Nil(t, err)

	jobRuns, err = store.JobRunsFor(job)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(jobRuns))
	jobRun := jobRuns[0]
	assert.Equal(t, expectedResponse, jobRun.TaskRuns[0].Result.Value())
	jobRun = jobRuns[0]
	assert.Equal(t, "10583.75", jobRun.TaskRuns[1].Result.Value())
}

func TestCreateInvalidJobs(t *testing.T) {
	store := cltest.Store()
	defer store.Close()
	server := cltest.SetUpWeb(store)
	defer cltest.TearDownWeb()

	jsonStr := cltest.LoadJSON("./fixtures/create_invalid_jobs.json")
	resp, err := http.Post(server.URL+"/jobs", "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 500, resp.StatusCode, "Response should be internal error")

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	assert.Equal(t, `{"errors":["IdoNotExist is not a supported adapter type"]}`, string(body), "Response should return JSON")
}

func TestCreateInvalidCron(t *testing.T) {
	store := cltest.Store()
	defer store.Close()
	server := cltest.SetUpWeb(store)
	defer cltest.TearDownWeb()

	jsonStr := cltest.LoadJSON("./fixtures/create_invalid_cron.json")
	resp, err := http.Post(server.URL+"/jobs", "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 500, resp.StatusCode, "Response should be internal error")

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	assert.Equal(t, `{"errors":["Cron: Failed to parse int from !: strconv.Atoi: parsing \"!\": invalid syntax"]}`, string(body), "Response should return JSON")
}

func TestShowJobs(t *testing.T) {
	store := cltest.Store()
	defer store.Close()
	server := cltest.SetUpWeb(store)
	defer cltest.TearDownWeb()

	j := models.NewJob()
	j.Schedule = models.Schedule{Cron: "9 9 9 9 6"}

	store.Save(&j)

	resp, err := http.Get(server.URL + "/jobs/" + j.ID)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode, "Response should be successful")
	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()

	var respJob models.Job
	json.Unmarshal(b, &respJob)
	assert.Equal(t, respJob.Schedule, j.Schedule, "should have the same schedule")
}

func TestShowNotFoundJobs(t *testing.T) {
	store := cltest.Store()
	defer store.Close()
	server := cltest.SetUpWeb(store)
	defer cltest.TearDownWeb()

	resp, err := http.Get(server.URL + "/jobs/" + "garbage")
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode, "Response should be not found")
}
