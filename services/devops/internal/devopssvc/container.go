package devopssvc

import (
	"context"
	"path/filepath"
	"strings"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

func (s *Server) BuildImage(ctx context.Context, req *devopsv1.BuildImageRequest) (*devopsv1.BuildImageResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	dockerfile, err := s.workspaces.ContainedPath(root, req.GetDockerfile(), "Dockerfile")
	if err != nil {
		return nil, operationError(err)
	}
	tag := firstNonBlank(req.GetTag(), filepath.Base(root)+":dev")
	plan := mutationPlan("container.build", tag, req.GetReason(), true, "container.build")
	if req.GetDryRun() {
		return &devopsv1.BuildImageResponse{Plan: plan, Image: &devopsv1.Artifact{Name: tag, Kind: "container-image"}}, nil
	}
	result, err := s.commands.Run(ctx, root, "docker", "build", "-f", dockerfile, "-t", tag, ".")
	if err != nil {
		return nil, operationError(err)
	}
	if result.GetStatus() != taskStatusPassed {
		return nil, serviceerrors.Internalf("docker build failed: %s", strings.Join(result.GetLogs(), "\n"))
	}
	return &devopsv1.BuildImageResponse{Plan: plan, Image: &devopsv1.Artifact{Name: tag, Kind: "container-image", Uri: tag}}, nil
}

func (s *Server) ListImages(ctx context.Context, req *devopsv1.ListImagesRequest) (*devopsv1.ListImagesResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 20
	}
	out, err := s.commands.Run(ctx, "", "docker", "images", "--format", "{{.ID}}\t{{.Repository}}\t{{.Tag}}\t{{.Digest}}")
	if err != nil {
		return nil, operationError(err)
	}
	if out.GetStatus() != taskStatusPassed {
		return nil, serviceerrors.Unavailable("docker image inventory is unavailable")
	}
	filter := strings.ToLower(strings.TrimSpace(req.GetFilter()))
	images := make([]*devopsv1.ContainerImage, 0)
	for _, line := range nonEmptyLines(strings.Join(out.GetLogs(), "\n")) {
		if filter != "" && !strings.Contains(strings.ToLower(line), filter) {
			continue
		}
		parts := strings.Split(line, "\t")
		for len(parts) < 4 {
			parts = append(parts, "")
		}
		images = append(images, &devopsv1.ContainerImage{Id: parts[0], Repository: parts[1], Tag: parts[2], Digest: parts[3]})
		if len(images) >= limit {
			break
		}
	}
	return &devopsv1.ListImagesResponse{Images: images}, nil
}

func (s *Server) PlanRun(_ context.Context, req *devopsv1.PlanRunRequest) (*devopsv1.PlanRunResponse, error) {
	if strings.TrimSpace(req.GetImage()) == "" {
		return nil, serviceerrors.InvalidArgument("image is required")
	}
	return &devopsv1.PlanRunResponse{Plan: mutationPlan("container.run", req.GetImage(), "run container with selected args/env", true, "container.run")}, nil
}
