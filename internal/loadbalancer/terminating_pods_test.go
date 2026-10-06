package loadbalancer

import (
	"context"
	"testing"
	"time"

	v1 "github.com/kubeai-project/kubeai/api/k8s/v1"
	"github.com/kubeai-project/kubeai/internal/apiutils"
	"github.com/kubeai-project/kubeai/internal/metrics/metricstest"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func readyPodForRoutingTest(name, ip string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "default",
			Labels:      map[string]string{v1.PodModelLabel: "decision"},
			Annotations: map[string]string{v1.ModelPodPortAnnotation: "8000"},
			Finalizers:  []string{"test.kubeai.org/hold-deletion"},
		},
		Status: corev1.PodStatus{
			PodIP:      ip,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
}

func routingTestClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestReconcileStopsRoutingToTerminatingPods(t *testing.T) {
	for _, strategy := range []v1.LoadBalancingStrategy{v1.LeastLoadStrategy, v1.PrefixHashStrategy} {
		t.Run(string(strategy), func(t *testing.T) {
			metricstest.Init(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			lb := v1.LoadBalancing{Strategy: strategy, PrefixHash: v1.PrefixHash{Replication: 10, MeanLoadPercentage: 125}}
			model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "decision", Namespace: "default"}, Spec: v1.ModelSpec{LoadBalancing: lb}}
			draining := readyPodForRoutingTest("draining", "10.0.0.1")
			healthy := readyPodForRoutingTest("healthy", "10.0.0.2")
			r := &LoadBalancer{Client: routingTestClient(t, model, draining), groups: map[string]*group{}}
			reconcile := func(p *corev1.Pod) {
				t.Helper()
				_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(p)})
				require.NoError(t, err)
			}
			reconcile(draining)
			request := &apiutils.Request{Model: model.Name, LoadBalancing: lb}
			address, finishOld, err := r.AwaitBestAddress(ctx, request)
			require.NoError(t, err)
			require.Equal(t, "10.0.0.1:8000", address)
			g, ok := r.getEndpointGroup(model.Name)
			require.True(t, ok)
			t.Cleanup(func() {
				finishOld()
				require.Zero(t, g.totalInFlight.Load())
			})
			require.NoError(t, r.Create(ctx, healthy))
			require.NoError(t, r.Delete(ctx, draining))
			var got corev1.Pod
			require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(draining), &got))
			require.NotNil(t, got.DeletionTimestamp)
			require.Equal(t, corev1.ConditionTrue, got.Status.Conditions[0].Status)
			reconcile(draining)
			require.Equal(t, []string{"10.0.0.2:8000"}, r.GetAllAddresses(model.Name))
			address, finishNew, err := r.AwaitBestAddress(ctx, request)
			require.NoError(t, err)
			require.Equal(t, "10.0.0.2:8000", address)
			finishNew()
			require.EqualValues(t, 1, g.totalInFlight.Load(), "the draining request remains accounted for")
			require.NoError(t, r.Delete(ctx, healthy))
			reconcile(healthy)
			require.Empty(t, r.GetAllAddresses(model.Name))
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			_, _, err = r.AwaitBestAddress(cancelled, request)
			require.ErrorIs(t, err, context.Canceled)
		})
	}
}

func TestReconcileRemovesAlreadyDeletedPod(t *testing.T) {
	for _, strategy := range []v1.LoadBalancingStrategy{v1.LeastLoadStrategy, v1.PrefixHashStrategy} {
		t.Run(string(strategy), func(t *testing.T) {
			metricstest.Init(t)
			lb := v1.LoadBalancing{Strategy: strategy, PrefixHash: v1.PrefixHash{Replication: 10, MeanLoadPercentage: 125}}
			r := &LoadBalancer{Client: routingTestClient(t), groups: map[string]*group{}}
			g := r.getOrCreateEndpointGroup("decision", lb)
			g.reconcileEndpoints(map[string]endpoint{
				"default/deleted": {address: "10.0.0.1:8000"},
				"other/deleted":   {address: "10.0.0.2:8000"},
			})
			r.getOrCreateEndpointGroup("unrelated", lb).reconcileEndpoints(map[string]endpoint{
				"default/healthy": {address: "10.0.0.3:8000"},
			})
			for i := 0; i < 2; i++ {
				_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Namespace: "default", Name: "deleted"}})
				require.NoError(t, err)
			}
			require.Equal(t, []string{"10.0.0.2:8000"}, r.GetAllAddresses("decision"))
			require.Equal(t, []string{"10.0.0.3:8000"}, r.GetAllAddresses("unrelated"))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			address, finish, err := r.AwaitBestAddress(ctx, &apiutils.Request{Model: "decision", LoadBalancing: lb})
			require.NoError(t, err)
			require.Equal(t, "10.0.0.2:8000", address)
			finish()
			require.Zero(t, g.totalInFlight.Load())
		})
	}
}

func TestReconcileExcludesTerminatingKubeAIPeer(t *testing.T) {
	healthy := readyPodForRoutingTest("kubeai-healthy", "10.0.0.1")
	draining := readyPodForRoutingTest("kubeai-draining", "10.0.0.2")
	for _, p := range []*corev1.Pod{healthy, draining} {
		p.Labels = map[string]string{"app.kubernetes.io/name": "kubeai"}
	}
	r := &LoadBalancer{Client: routingTestClient(t, healthy, draining), groups: map[string]*group{}}
	require.NoError(t, r.Delete(context.Background(), draining))
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(healthy)})
	require.NoError(t, err)
	require.Equal(t, []string{"10.0.0.1"}, r.GetSelfIPs())
}
