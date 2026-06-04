// Isopace SQL store adapter — a separate module so the stdlib-only core never
// gains a database dependency. The adapter itself imports only database/sql;
// the integrator supplies the driver.
module github.com/teqpace-services/isopace/adapters/sql

go 1.26

require github.com/teqpace-services/isopace v0.3.0

// In-repo builds (and CI building this module from a checkout) resolve the core
// from the working tree. External consumers ignore replace and use the require
// above.
replace github.com/teqpace-services/isopace => ../..
