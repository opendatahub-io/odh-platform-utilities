package flakiness

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"time"
)

// JUnit XML property name constants for enriched failure classification.
const (
	PropertyFailureCategory    = "failure.category"
	PropertyFailureSubcategory = "failure.subcategory"
	PropertyFailureConfidence  = "failure.confidence"
)

// JUnitTestSuites is the top-level <testsuites> wrapper.
type JUnitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite maps a single <testsuite>.
type JUnitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Time      float64         `xml:"time,attr"`
	Timestamp string          `xml:"timestamp,attr"`
	Cases     []JUnitTestCase `xml:"testcase"`
}

// JUnitTestCase maps a single <testcase>.
type JUnitTestCase struct {
	Name       string          `xml:"name,attr"`
	ClassName  string          `xml:"classname,attr"`
	Time       float64         `xml:"time,attr"`
	Failure    *JUnitFailure   `xml:"failure"`
	Error      *JUnitError     `xml:"error"`
	Skipped    *JUnitSkipped   `xml:"skipped"`
	Properties []JUnitProperty `xml:"properties>property"`
}

// JUnitFailure maps a <failure>.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// JUnitError maps an <error>.
type JUnitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// JUnitSkipped maps a <skipped>.
type JUnitSkipped struct {
	Message string `xml:"message,attr"`
}

// JUnitProperty maps a <property> within <properties>.
type JUnitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// ParseJUnit handles both <testsuites> (wrapping multiple suites)
// and bare <testsuite> documents.
func ParseJUnit(data []byte) ([]JUnitTestSuite, error) {
	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err == nil {
		if len(suites.Suites) > 0 {
			return suites.Suites, nil
		}

		// Valid <testsuites> with zero children, return empty rather
		// than falling through to the bare-<testsuite> path.
		if suites.XMLName.Local == "testsuites" {
			return nil, nil
		}
	}

	var single JUnitTestSuite
	if err := xml.Unmarshal(data, &single); err != nil {
		return nil, fmt.Errorf("parsing JUnit XML: %w", err)
	}

	return []JUnitTestSuite{single}, nil
}

// ConvertTestResults turns parsed JUnit suites into [TestResult] values.
// The job and buildID must be supplied by the caller (not present in the XML).
func ConvertTestResults(suites []JUnitTestSuite, job, buildID string) []TestResult {
	results := make([]TestResult, 0, len(suites))

	for i := range suites {
		suite := &suites[i]
		suiteTS := parseSuiteTimestamp(suite.Timestamp)

		for j := range suite.Cases {
			tc := &suite.Cases[j]

			r := TestResult{
				Name:      tc.Name,
				Suite:     suite.Name,
				Job:       job,
				BuildID:   buildID,
				Result:    outcomeFromCase(tc),
				Duration:  time.Duration(tc.Time * float64(time.Second)),
				Timestamp: suiteTS,
			}

			applyFailureProperties(&r, tc.Properties)

			results = append(results, r)
		}
	}

	return results
}

func outcomeFromCase(tc *JUnitTestCase) TestOutcome {
	switch {
	case tc.Error != nil:
		return OutcomeError
	case tc.Failure != nil:
		return OutcomeFail
	case tc.Skipped != nil:
		return OutcomeSkip
	default:
		return OutcomePass
	}
}

func applyFailureProperties(r *TestResult, props []JUnitProperty) {
	for i := range props {
		switch props[i].Name {
		case PropertyFailureCategory:
			r.FailureCategory = props[i].Value
		case PropertyFailureSubcategory:
			r.FailureSubcategory = props[i].Value
		case PropertyFailureConfidence:
			r.FailureConfidence = props[i].Value
		}
	}
}

var suiteTimestampLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

func parseSuiteTimestamp(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}

	for _, layout := range suiteTimestampLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}

	if epoch, err := strconv.ParseFloat(raw, 64); err == nil {
		sec := int64(epoch)
		nsec := int64((epoch - float64(sec)) * 1e9)

		return time.Unix(sec, nsec)
	}

	return time.Time{}
}
