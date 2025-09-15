package reconcile

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nephio-project/porch/internal/kpt/errors"
	"github.com/nephio-project/porch/internal/kpt/options"
	"github.com/nephio-project/porch/internal/kpt/util/porch"
	"github.com/nephio-project/porch/pkg/cli/commands/repo/docs"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/cmd/get"
)

const (
	command      = "cmdreposync"
	resourceName = "repositories.config.porch.kpt.dev"
)

func NewCommand(ctx context.Context, rcg *genericclioptions.ConfigFlags) *cobra.Command {
	return newRunner(ctx, rcg).Command
}

func newRunner(ctx context.Context, rcg *genericclioptions.ConfigFlags) *runner {
	r := &runner{
		ctx:        ctx,
		getFlags:   options.Get{ConfigFlags: rcg},
		printFlags: get.NewGetPrintFlags(),
	}
	c := &cobra.Command{
		Use:     "reconcile [REPOSITORY_NAME]",
		Aliases: []string{"sync"},
		Short:   docs.SyncShort,
		Long:    docs.SyncShort + "\n" + docs.SyncLong,
		Example: docs.SyncExamples,
		PreRunE: r.preRunE,
		RunE:    r.runE,
		Hidden:  porch.HidePorchCommands,
	}
	r.Command = c

	r.getFlags.AddFlags(c)
	r.printFlags.AddFlags(c)

	// Add --all flag
	c.Flags().Bool("all", false, "reconcile all repositories in the namespace")

	return r
}

type runner struct {
	ctx     context.Context
	Command *cobra.Command

	getFlags   options.Get
	printFlags *get.PrintFlags

	requestTable bool
}

func (r *runner) preRunE(cmd *cobra.Command, _ []string) error {
	outputOption := cmd.Flags().Lookup("output").Value.String()
	r.requestTable = !(strings.Contains(outputOption, "custom-columns") || outputOption == "yaml" || strings.Contains(outputOption, "json"))

	allNamespacesFlag := cmd.Flags().Lookup("all-namespaces").Value.String()
	r.printFlags.HumanReadableFlags.WithNamespace = strings.Contains(allNamespacesFlag, "true")

	return nil
}

func (r *runner) runE(cmd *cobra.Command, args []string) error {
	const op errors.Op = command + ".runE"

	restConfig, err := r.getFlags.ToRESTConfig()
	if err != nil {
		return errors.E(op, err)
	}
	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return errors.E(op, err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "config.porch.kpt.dev",
		Version:  "v1alpha1",
		Resource: "repositories",
	}

	namespace := ""
	if r.getFlags.ConfigFlags.Namespace != nil {
		namespace = *r.getFlags.ConfigFlags.Namespace
	}
	if namespace == "" {
		namespace = "default"
	}

	allFlag := r.Command.Flags().Lookup("all").Value.String() == "true"
	allNamespaces := r.Command.Flags().Lookup("all-namespaces").Value.String() == "true"

	var repos []*unstructured.Unstructured

	if allFlag {
		var list *unstructured.UnstructuredList
		if allNamespaces {
			list, err = dynClient.Resource(gvr).List(r.ctx, metav1.ListOptions{})
		} else {
			list, err = dynClient.Resource(gvr).Namespace(namespace).List(r.ctx, metav1.ListOptions{})
		}
		if err != nil {
			return errors.E(op, err)
		}
		for i := range list.Items {
			repos = append(repos, &list.Items[i])
		}
	} else {
		if len(args) == 0 {
			return fmt.Errorf("repository name(s) required unless --all is set")
		}
		for _, repoName := range args {
			repo, err := dynClient.Resource(gvr).Namespace(namespace).Get(r.ctx, repoName, metav1.GetOptions{})
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error fetching repository %q: %v\n", repoName, err)
				continue
			}
			repos = append(repos, repo)
		}
	}

	if len(repos) == 0 {
		return fmt.Errorf("no repositories found to sync")
	}

	for _, repo := range repos {
		annotations := repo.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["porch.kpt.dev/sync"] = time.Now().Format("2006-01-02T15:04:05Z")
		repo.SetAnnotations(annotations)

		_, err := dynClient.Resource(gvr).Namespace(repo.GetNamespace()).Update(r.ctx, repo, metav1.UpdateOptions{})
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Failed to update repository %q in namespace %q: %v\n", repo.GetName(), repo.GetNamespace(), err)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Repository sync requested for %q in namespace %q\n", repo.GetName(), repo.GetNamespace())
	}

	return nil
}
