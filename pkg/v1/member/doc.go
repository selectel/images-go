/*
Package member provides typed operations on the members of a shared Glance v2 image: the
projects that were granted access to it and the status of that access.

The image owner creates and deletes members; the member project changes the status of its own
membership. Every operation performs a single request.
*/
package member
