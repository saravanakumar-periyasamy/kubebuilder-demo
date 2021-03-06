/*
Copyright 2019 The Upbound Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package job

import (
	"context"
	"fmt"
	batchv1alpha1 "github.com/saravanakumar-periyasamy/kubebuilder-demo/pkg/apis/batch/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"math/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

var log = logf.Log.WithName("controller")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

type agent struct {
	name         string
	jobsQueue    map[string]*batchv1alpha1.Job
	reconcileJob *ReconcileJob
}

var agents []*agent

func init() {
	agents = []*agent{
		{name: "agent1", jobsQueue: make(map[string]*batchv1alpha1.Job)},
		{name: "agent2", jobsQueue: make(map[string]*batchv1alpha1.Job)},
		{name: "agent3", jobsQueue: make(map[string]*batchv1alpha1.Job)},
	}
	for _, agent := range agents {
		go agent.processJobs()
	}
}

func (a *agent) processJobs() {
	for true {
		log.Info("processing jobsQueue...", "agent", a.name, "jobsQueue", len(a.jobsQueue))
		for key, job := range a.jobsQueue {
			if job.Status.State == "" {
				job.Status.State = batchv1alpha1.Pending
				err := a.reconcileJob.Update(context.TODO(), job)
				if err != nil {
					log.Error(err, "Error while updating the job")
				}
			} else if a.isReadyForProcessing(job) {
				log.Info("Processing job...", "agent", a.name, "job", job.Name)
				time.Sleep(time.Duration(rand.Intn(25) + 5))
				job.Spec.Result = rand.Int31n(100)
				if job.Spec.Result%2 == 0 {
					job.Status.State = batchv1alpha1.Succeeded
					//remove from the job list
					delete(a.jobsQueue, key)
					a.reconcileJob.recorder.Event(job, "Normal", "Succeeded", "Job Succeeded, result:"+fmt.Sprint(job.Spec.Result))
					log.Info("Job Succeeded", "job", job.Name, "result", job.Spec.Result)
				} else {
					job.Status.State = batchv1alpha1.Failed
					a.reconcileJob.recorder.Event(job, "Warning", "Failed", "Job Failed, result:"+fmt.Sprint(job.Spec.Result))
					log.Info("Job Failed", "job", job.Name, "result", job.Spec.Result)
				}
				err := a.reconcileJob.Update(context.TODO(), job)
				if err != nil {
					log.Error(err, "Error while updating the job")
				}
			}
		}
		time.Sleep(5 * time.Second)
	}
}

func (a *agent) isReadyForProcessing(job *batchv1alpha1.Job) bool {
	//If not job has not succeeded
	if job.Status.State != batchv1alpha1.Succeeded {
		for _, dependentJobName := range job.Spec.DependOnJobs {
			dependentJob, err := a.findJobByName(dependentJobName, job.Namespace)
			if err != nil {
				log.Error(err, "Error in finding job")
			}
			if dependentJob.Status.State != batchv1alpha1.Succeeded {
				log.Info("Dependent Job is not succeeded", "job", job.Name, "depedent-job", dependentJob.Name)
				a.reconcileJob.recorder.Event(job, "Warning", "Pending", "Dependent Job is not succeeded, job:"+dependentJob.Name)
				return false
			}
		}
		return true
	}
	return false
}

func (a *agent) findJobByName(name string, namespace string) (*batchv1alpha1.Job, error) {
	jobList := batchv1alpha1.JobList{}
	err := a.reconcileJob.List(context.TODO(), &client.ListOptions{Namespace: namespace}, &jobList)
	if err != nil {
		return nil, err
	}
	for _, job := range jobList.Items {
		if job.Name == name {
			return &job, nil
		}
	}
	return nil, fmt.Errorf("could not find the job %v in %v namespace", name, namespace)
}

// Add creates a new Job Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileJob{Client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetRecorder("job-controller")}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("job-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Job
	err = c.Watch(&source.Kind{Type: &batchv1alpha1.Job{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileJob{}

// ReconcileJob reconciles a Job object
type ReconcileJob struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// A simple scheduler that picks the agents randomly
func (r *ReconcileJob) scheduleJobToAnAgent(job *batchv1alpha1.Job) string {
	agent := agents[rand.Intn(len(agents))]
	//job.Status.State = batchv1alpha1.Pending
	agent.jobsQueue[job.Namespace+"/"+job.Name] = job
	agent.reconcileJob = r
	//err := r.Status().Update(context.TODO(), job)
	//if err != nil {
	//	log.Error(err, "scheduleJobToAnAgent: Error while updating the job", "job", job.Name, "namespace", job.Namespace)
	//}
	r.recorder.Event(job, "Normal", "Pending", "Job Pending")
	return agent.name
}

// Reconcile reads that state of the cluster for a Job object and makes changes based on the state read
// and what is in the Job.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch.crossplane.io,resources=jobsQueue,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch.crossplane.io,resources=jobsQueue/status,verbs=get;update;patch
func (r *ReconcileJob) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Job instance
	instance := &batchv1alpha1.Job{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.Spec.Agent == "" {
		instance.Spec.Agent = r.scheduleJobToAnAgent(instance)
		log.Info(instance.Spec.Agent)
		err := r.Update(context.TODO(), instance)
		if err != nil {
			log.Error(err, "Error while updating the resource")
		}
	}
	return reconcile.Result{}, nil
}
