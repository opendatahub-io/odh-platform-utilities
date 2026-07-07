package flakiness_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

func TestParseJUnit_Testsuites(t *testing.T) {
	t.Parallel()

	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="e2e" tests="3" failures="1" errors="0" time="17.2" timestamp="2026-06-24T12:00:00Z">
    <testcase name="TestModelServing/basic_inference" classname="e2e" time="5.2">
    </testcase>
    <testcase name="TestDashboard/login" classname="e2e" time="10.0">
      <failure message="expected 200 got 500" type="AssertionError">stack trace</failure>
      <properties>
        <property name="failure.category" value="product"/>
        <property name="failure.subcategory" value="api_error"/>
        <property name="failure.confidence" value="0.95"/>
      </properties>
    </testcase>
    <testcase name="TestPipelines/gpu" classname="e2e" time="0">
      <skipped message="GPU not available"/>
    </testcase>
  </testsuite>
</testsuites>`)

	suites, err := flakiness.ParseJUnit(data)
	require.NoError(t, err)
	require.Len(t, suites, 1)

	suite := suites[0]
	assert.Equal(t, "e2e", suite.Name)
	assert.Equal(t, 3, suite.Tests)
	assert.Equal(t, 1, suite.Failures)
	require.Len(t, suite.Cases, 3)

	assert.Equal(t, "TestModelServing/basic_inference", suite.Cases[0].Name)
	assert.Nil(t, suite.Cases[0].Failure)
	assert.Nil(t, suite.Cases[0].Skipped)

	assert.Equal(t, "TestDashboard/login", suite.Cases[1].Name)
	require.NotNil(t, suite.Cases[1].Failure)
	assert.Equal(t, "expected 200 got 500", suite.Cases[1].Failure.Message)
	require.Len(t, suite.Cases[1].Properties, 3)

	assert.Equal(t, "TestPipelines/gpu", suite.Cases[2].Name)
	require.NotNil(t, suite.Cases[2].Skipped)
}

func TestParseJUnit_BareSuite(t *testing.T) {
	t.Parallel()

	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="unit" tests="1" failures="0" errors="0" time="0.5">
  <testcase name="TestFoo" classname="pkg" time="0.5"/>
</testsuite>`)

	suites, err := flakiness.ParseJUnit(data)
	require.NoError(t, err)
	require.Len(t, suites, 1)
	assert.Equal(t, "unit", suites[0].Name)
	require.Len(t, suites[0].Cases, 1)
	assert.Equal(t, "TestFoo", suites[0].Cases[0].Name)
}

func TestParseJUnit_ErrorElement(t *testing.T) {
	t.Parallel()

	data := []byte(`<testsuite name="e2e" tests="1">
  <testcase name="TestCrash" classname="e2e" time="1.0">
    <error message="panic" type="RuntimeError">goroutine dump</error>
  </testcase>
</testsuite>`)

	suites, err := flakiness.ParseJUnit(data)
	require.NoError(t, err)
	require.Len(t, suites, 1)
	require.Len(t, suites[0].Cases, 1)
	require.NotNil(t, suites[0].Cases[0].Error)
	assert.Equal(t, "panic", suites[0].Cases[0].Error.Message)
}

func TestParseJUnit_InvalidXML(t *testing.T) {
	t.Parallel()

	_, err := flakiness.ParseJUnit([]byte(`not xml at all`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing JUnit XML")
}

func TestConvertTestResults(t *testing.T) { //nolint:funlen // Table-driven test with thorough assertions.
	t.Parallel()

	suites := []flakiness.JUnitTestSuite{
		{
			Name:      "e2e",
			Timestamp: "2026-06-24T12:00:00Z",
			Cases: []flakiness.JUnitTestCase{
				{Name: "TestPass", Time: 5.2},
				{
					Name:    "TestFail",
					Time:    10.0,
					Failure: &flakiness.JUnitFailure{Message: "bad"},
					Properties: []flakiness.JUnitProperty{
						{Name: "failure.category", Value: "infrastructure"},
						{Name: "failure.subcategory", Value: "timeout"},
						{Name: "failure.confidence", Value: "0.87"},
					},
				},
				{
					Name:  "TestError",
					Time:  1.0,
					Error: &flakiness.JUnitError{Message: "panic"},
				},
				{
					Name:    "TestSkip",
					Time:    0,
					Skipped: &flakiness.JUnitSkipped{Message: "no GPU"},
				},
			},
		},
	}

	results := flakiness.ConvertTestResults(suites, "periodic-ci-main", "build-42")

	require.Len(t, results, 4)

	expectedTS := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	pass := results[0]
	assert.Equal(t, "TestPass", pass.Name)
	assert.Equal(t, "e2e", pass.Suite)
	assert.Equal(t, "periodic-ci-main", pass.Job)
	assert.Equal(t, "build-42", pass.BuildID)
	assert.Equal(t, flakiness.OutcomePass, pass.Result)
	assert.InDelta(t, 5.2, pass.Duration.Seconds(), 0.001)
	assert.Equal(t, expectedTS, pass.Timestamp)
	assert.Empty(t, pass.FailureCategory)

	fail := results[1]
	assert.Equal(t, flakiness.OutcomeFail, fail.Result)
	assert.Equal(t, "infrastructure", fail.FailureCategory)
	assert.Equal(t, "timeout", fail.FailureSubcategory)
	assert.Equal(t, "0.87", fail.FailureConfidence)

	errResult := results[2]
	assert.Equal(t, flakiness.OutcomeError, errResult.Result)

	skip := results[3]
	assert.Equal(t, flakiness.OutcomeSkip, skip.Result)
}

func TestConvertTestResults_MissingTimestamp(t *testing.T) {
	t.Parallel()

	suites := []flakiness.JUnitTestSuite{
		{
			Name: "unit",
			Cases: []flakiness.JUnitTestCase{
				{Name: "TestFoo", Time: 1.0},
			},
		},
	}

	results := flakiness.ConvertTestResults(suites, "ci", "b1")
	require.Len(t, results, 1)
	assert.True(t, results[0].Timestamp.IsZero())
}

func TestConvertTestResults_EmptyInput(t *testing.T) {
	t.Parallel()

	results := flakiness.ConvertTestResults(nil, "ci", "b1")
	assert.Empty(t, results)

	results = flakiness.ConvertTestResults([]flakiness.JUnitTestSuite{}, "ci", "b1")
	assert.Empty(t, results)
}

func TestParseJUnit_EmptyTestsuites(t *testing.T) {
	t.Parallel()

	data := []byte(`<testsuites></testsuites>`)

	suites, err := flakiness.ParseJUnit(data)
	require.NoError(t, err)
	assert.Empty(t, suites)
}

func TestParseJUnit_MultipleSuites(t *testing.T) {
	t.Parallel()

	data := []byte(`<testsuites>
  <testsuite name="suite-a" tests="1"><testcase name="TestA" time="1"/></testsuite>
  <testsuite name="suite-b" tests="1"><testcase name="TestB" time="2"/></testsuite>
</testsuites>`)

	suites, err := flakiness.ParseJUnit(data)
	require.NoError(t, err)
	require.Len(t, suites, 2)
	assert.Equal(t, "suite-a", suites[0].Name)
	assert.Equal(t, "suite-b", suites[1].Name)
}
