// Command database-connection-condition demonstrates a component reporting
// database connectivity on its own custom resource using the
// connection-condition helpers from pkg/database/conditions.
//
// The example covers four scenarios:
//
//  1. A successful connection sets DatabaseConnected=True.
//  2. A network failure sets DatabaseConnected=False with the reason
//     classified from the error.
//  3. An authentication failure with a DSN-embedded password sets
//     DatabaseConnected=False with an explicitly supplied reason and the
//     message redacted.
//  4. An opt-in condition Manager aggregates DatabaseConnected into the
//     Ready condition.
//
// It uses a stub database/sql driver so both outcomes can be demonstrated
// without a PostgreSQL server or a driver dependency.
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	kcond "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/database/conditions"
)

var (
	errStubPrepare = errors.New("prepare is not supported by the stub driver")
	errStubBegin   = errors.New("transactions are not supported by the stub driver")

	// errDialRefused is the network-level failure used by scenario 2.
	errDialRefused = &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: syscall.ECONNREFUSED,
	}

	// errAuthRejected mimics a driver error whose message embeds the DSN
	// together with its password.
	errAuthRejected = errors.New("failed to connect to " +
		"`host=db.example.com user=modelregistry password=s3cr3t dbname=registry`: " +
		`server error: FATAL: password authentication failed for user "modelregistry" (SQLSTATE 28P01)`)
)

// stubDriver is a database/sql driver whose Ping returns a configurable
// error, so the example can demonstrate both connection outcomes without a
// real PostgreSQL server.
type stubDriver struct {
	pingErr error
}

// Open returns a connection immediately; the stub never dials anything.
func (d *stubDriver) Open(string) (driver.Conn, error) {
	return stubConn{driver: d}, nil
}

// stubConn implements the minimal driver.Conn and driver.Pinger interfaces.
type stubConn struct {
	driver *stubDriver
}

func (stubConn) Prepare(string) (driver.Stmt, error) {
	return nil, errStubPrepare
}

func (stubConn) Close() error {
	return nil
}

func (stubConn) Begin() (driver.Tx, error) {
	return nil, errStubBegin
}

// Ping reports the configured connection outcome.
func (c stubConn) Ping(context.Context) error {
	return c.driver.pingErr
}

// exampleComponent is a minimal component CR. Embedding common.Status
// provides the ConditionsAccessor interface the helpers operate on; real
// modules implement the full common.PlatformObject contract.
type exampleComponent struct {
	common.Status
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	drv := &stubDriver{}
	sql.Register("condition-demo", drv)

	// In a real controller the condition is persisted with the status
	// subresource after the in-memory mutation, typically via pkg/status:
	//
	//	err := status.Update(ctx, client, obj, func(o *ExampleComponent) {
	//		conditions.SetConnected(o)
	//	})

	err := scenarioSuccess(drv)
	if err != nil {
		return fmt.Errorf("scenario success: %w", err)
	}

	err = scenarioNetworkFailure(drv)
	if err != nil {
		return fmt.Errorf("scenario network failure: %w", err)
	}

	err = scenarioAuthFailure(drv)
	if err != nil {
		return fmt.Errorf("scenario auth failure: %w", err)
	}

	return scenarioManager()
}

// ping opens a connection through the registered stub driver and reports
// the ping result, mimicking a component checking its conditions.
func ping() error {
	db, err := sql.Open("condition-demo", "")
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	return db.PingContext(ctx)
}

func scenarioSuccess(drv *stubDriver) error {
	drv.pingErr = nil

	comp := &exampleComponent{}

	err := ping()
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	conditions.SetConnected(comp,
		kcond.WithMessage("connected to db.example.com:5432/model_registry"),
	)

	return printStatus("scenario 1: successful connection", comp)
}

func scenarioNetworkFailure(drv *stubDriver) error {
	drv.pingErr = errDialRefused

	comp := &exampleComponent{}

	err := ping()
	if err != nil {
		// ReasonForError classifies the network failure; the message
		// carries the error text.
		conditions.SetFailed(comp, err)
	}

	return printStatus("scenario 2: network failure (reason classified)", comp)
}

func scenarioAuthFailure(drv *stubDriver) error {
	drv.pingErr = errAuthRejected

	comp := &exampleComponent{}

	err := ping()
	if err != nil {
		// Standard library inspection cannot detect an authentication
		// failure, so the reason is supplied explicitly.
		conditions.SetFailed(comp, err,
			kcond.WithReason(common.ReasonCredentialsRejected),
		)
	}

	return printStatus("scenario 3: auth failure (reason explicit, message redacted)", comp)
}

func scenarioManager() error {
	comp := &exampleComponent{}

	// The Manager is an opt-in alternative to the raw helpers: it uses the
	// same constants from api/common, but goes through MarkFalse and
	// MarkTrue so that aggregation into Ready happens automatically. Do not
	// mix the two paths on the same object: the raw helpers upsert the
	// condition without triggering the Manager's recomputation.
	mgr := kcond.NewManager(comp,
		string(common.ConditionTypeReady),
		string(common.ConditionTypeDatabaseConnected),
	)

	mgr.MarkFalse(
		string(common.ConditionTypeDatabaseConnected),
		kcond.WithReason(common.ReasonHostUnreachable),
		kcond.WithMessage("dial tcp: connection refused"),
	)
	mgr.Sort()

	err := printStatus("scenario 4a: manager aggregates failure into ready", comp)
	if err != nil {
		return err
	}

	mgr.MarkTrue(string(common.ConditionTypeDatabaseConnected),
		kcond.WithReason(common.ReasonConnectionSucceeded),
	)
	mgr.Sort()

	return printStatus("scenario 4b: manager recovers to ready", comp)
}

func printStatus(title string, comp *exampleComponent) error {
	out, err := json.MarshalIndent(comp.Status, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling status: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "--- %s ---\n%s\n\n", title, out)

	return nil
}
