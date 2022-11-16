/*
Copyright 2022.

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

package controllers

import (
	"context"

	"github.com/redhat-appstudio/service-provider-integration-operator/pkg/spi-shared/tokenstorage"
	"k8s.io/apimachinery/pkg/types"

	//"k8s.io/apimachinery/pkg/labels"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	spi "github.com/redhat-appstudio/service-provider-integration-operator/api/v1beta1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const tokenSeretLabel = "spi.appstudio.redhat.com/token"

// SecretReconciler reconciles a Secret object
type SecretReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	TokenStorage tokenstorage.TokenStorage
}

//+kubebuilder:rbac:groups=core.github.com,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core.github.com,resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core.github.com,resources=secrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Secret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	list := &corev1.SecretList{}

	err := r.List(context.TODO(), list, client.HasLabels{tokenSeretLabel})
	if err != nil {
		log.Log.Error(err, "Can not get list of secrets ")
	}

	for _, s := range list.Items {

		var accessToken spi.SPIAccessToken

		err := r.Delete(context.TODO(), &s)
		if err != nil {
			logError(s.Name, req.Namespace, err, r, "can not delete Secret ")
			return ctrl.Result{}, err
		}

		if len(s.Data["spiTokenName"]) > 0 {
			accessToken = spi.SPIAccessToken{}
			err = r.Get(context.TODO(), types.NamespacedName{Name: string(s.Data["spiTokenName"]), Namespace: s.Namespace}, &accessToken)

			if err != nil {
				logError(s.Name, req.Namespace, err, r, "can not find SPI access token "+string(s.Data["spiTokenName"]))
				return ctrl.Result{}, err
			} else {
				log.Log.Info("SPI Access Token found : " + accessToken.Name)
			}

		} else if len(s.Data["providerUrl"]) > 0 {

			tkn := findTokenByUrl(string(s.Data["providerUrl"]), r)
			if tkn == nil {

				log.Log.Info("can not find SPI access token trying to create new one")
				accessToken = spi.SPIAccessToken{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "generated-spi-access-token-",
						Namespace:    s.Namespace,
					},
					Spec: spi.SPIAccessTokenSpec{
						ServiceProviderUrl: string(s.Data["providerUrl"]),
					},
				}
				err = r.Create(context.TODO(), &accessToken)
				if err != nil {
					logError(s.Name, req.Namespace, err, r, " can not create SPI access token for "+string(s.Data["providerUrl"]))
					return ctrl.Result{}, err
				} else {
					log.Log.Info("SPI Access Token created : " + accessToken.Name)
				}
			} else {
				accessToken = *tkn
				log.Log.Info("SPI Access Token found by providerUrl : " + accessToken.Name)
			}

		} else {
			logError(s.Name, req.Namespace, err, r, "Secret is invalid, neither spiTokenName nor providerUrl key found")
			return ctrl.Result{}, err
		}

		//log.Log.Info("token data : ", "userName", string(s.Data["userName"]), "Token", string(s.Data["tokenData"]))

		token := spi.Token{
			Username:    string(s.Data["userName"]),
			AccessToken: string(s.Data["tokenData"]),
		}

		err = r.TokenStorage.Store(ctx, &accessToken, &token)
		if err != nil {
			logError(s.Name, req.Namespace, err, r, "store failed ")
			return ctrl.Result{}, err
		}

		tryDeleteEvent(s.Name, req.Namespace, r)

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		Complete(r)
}

func logError(secretName string, ns string, err error, r *SecretReconciler, msg string) {

	tryDeleteEvent(secretName, ns, r)

	if err != nil {
		event := &corev1.Event{}
		//event.GenerateName = "Secret-"
		event.Name = secretName
		event.Message = msg
		event.Namespace = ns
		//event.Source = corev1.EventSource{}
		//event.InvolvedObject = corev1.ObjectReference{Namespace: req.Namespace, Name: s.Name, Kind: s.Kind, APIVersion: s.APIVersion}
		event.Type = "Error"
		event.Labels = map[string]string{
			"secretRef": secretName,
		}
		//event.EventTime = time.Now()

		err = r.Create(context.TODO(), event)

		log.Log.Error(err, msg)
		if err != nil {
			log.Log.Error(err, "Event creation failed ")
		}
	}

}

func tryDeleteEvent(secretName string, ns string, r *SecretReconciler) {
	stored := &corev1.Event{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: ns}, stored)

	if err == nil {

		log.Log.Info("event Found and will be deleted: " + stored.Name)
		err = r.Delete(context.TODO(), stored)
		if err != nil {
			log.Log.Error(err, " can not delete Event ")
		}

	}
}

func findTokenByUrl(url string, r *SecretReconciler) *spi.SPIAccessToken {

	list := spi.SPIAccessTokenList{}
	err := r.List(context.TODO(), &list)
	if err != nil {
		log.Log.Error(err, "Can not get list of tokens ")
		return nil
	}

	for _, t := range list.Items {

		if t.Spec.ServiceProviderUrl == url {
			return &t
		}

	}

	return nil
}
