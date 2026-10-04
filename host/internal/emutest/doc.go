// Package emutest holds the host's tests against the Android emulator (M2.5
// spec §7): they reach the app over adb, as the shell user, the way the host
// reaches a phone. They skip without ADBUDDY_EMULATOR_SERIAL (scripts/emulator.sh
// start prints it) and refuse any serial that is not an emulator: they never
// run on a real phone. TestInstrumentedSuite also runs the app's instrumented
// tests (am instrument, never Gradle's connected* tasks). The one exception is
// TestAppWorkflowPathsCoverDependencies, which needs no emulator and never
// skips: it checks the path filter of .github/workflows/emulator-app.yml.
package emutest
