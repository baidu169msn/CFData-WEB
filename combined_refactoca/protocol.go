package main

type githubUploadRequest struct {
	Token   string
	Owner   string
	Repo    string
	Branch  string
	Path    string
	Message string
	Content string
}
