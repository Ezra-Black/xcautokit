# Simulator fixture

Small UIKit application for opt-in XCAutokit integration checks. It deliberately
includes duplicate labels, a disabled control, delayed UI updates, text input, a
long-press recognizer, and an app-local permission-shaped alert. It requests no
system permissions and uses no network or external data.

Copy this directory to a temporary folder, generate the Xcode project with
`xcodegen generate`, then build/install/run using `build_run_sim` and the
`AutokitFixture` scheme on an explicitly selected spare simulator. Use
`scripts/mcp_live_test.py` for the integration checks. The live checks change
only that simulator's fixture UI; callers remain responsible for provisioning
and shutting down their chosen simulator.
