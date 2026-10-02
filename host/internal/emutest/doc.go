// Package emutest holds the host's tests against the Android emulator (M2.5
// spec §7): they reach the app over adb, as the shell user, the way the host
// reaches a phone. They skip without ADBUDDY_EMULATOR_SERIAL (scripts/emulator.sh
// start prints it) and refuse any serial that is not an emulator: they never
// run on a real phone. TestInstrumentedSuite also runs the app's instrumented
// tests (am instrument, never Gradle's connected* tasks).
package emutest
