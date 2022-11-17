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
	"time"

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

func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	list := &corev1.SecretList{}

	err := r.List(context.TODO(), list, client.HasLabels{tokenSeretLabel})
	if err != nil {
		log.Log.Error(err, "Can not get list of secrets ")
		return ctrl.Result{}, err
	}

	for _, s := range list.Items {

		var accessToken spi.SPIAccessToken

		// we immediatelly delete the Secret
		err := r.Delete(context.TODO(), &s)
		if err != nil {
			logError(s, err, r, "can not delete the Secret ")
			return ctrl.Result{}, err
		}

		// if spiTokenName field is not empty - try to find SPIAccessToken by it
		if len(s.Data["spiTokenName"]) > 0 {
			accessToken = spi.SPIAccessToken{}
			err = r.Get(context.TODO(), types.NamespacedName{Name: string(s.Data["spiTokenName"]), Namespace: s.Namespace}, &accessToken)

			if err != nil {
				logError(s, err, r, "can not find SPI access token "+string(s.Data["spiTokenName"]))
				return ctrl.Result{}, err
			} else {
				log.Log.Info("SPI Access Token found : " + accessToken.Name)
			}
			// spiTokenName field is empty
			// check providerUrl field and if not empty - try to find the token for this provider instance

		} else if len(s.Data["providerUrl"]) > 0 {

			// NOTE: it does not fit advanced policy of matching token!
			// Do we need it as an SPI "API function" which take into account this policy?
			tkn := findTokenByUrl(string(s.Data["providerUrl"]), r)
			// create new SPIAccessToken if there are no for such provider instance (URL)
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
					logError(s, err, r, " can not create SPI access token for "+string(s.Data["providerUrl"]))
					return ctrl.Result{}, err
				} else {
					// this is the only place where we can get the name of just created SPIAccessToken
					// which is presumably OK since SPI (binding) controller will look for the token by type/URL ?
					log.Log.Info("SPI Access Token created : " + accessToken.Name)
				}
			} else {
				accessToken = *tkn
				log.Log.Info("SPI Access Token found by providerUrl : " + accessToken.Name)
			}

		} else {
			logError(s, err, r, "Secret is invalid, neither spiTokenName nor providerUrl key found")
			return ctrl.Result{}, err
		}

		//log.Log.Info("token data : ", "userName", string(s.Data["userName"]), "Token", string(s.Data["tokenData"]))

		token := spi.Token{
			Username:    string(s.Data["userName"]),
			AccessToken: string(s.Data["tokenData"]),
		}

		err = r.TokenStorage.Store(ctx, &accessToken, &token)
		if err != nil {
			logError(s, err, r, "store failed ")
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

func logError(secret corev1.Secret, err error, r *SecretReconciler, msg string) {

	log.Log.Info("Error Event for Secret: " + secret.Name)

	tryDeleteEvent(secret.Name, secret.Namespace, r)

	if err != nil {
		event := &corev1.Event{}
		event.Name = secret.Name
		event.Message = msg
		event.Namespace = secret.Namespace
		event.Reason = "Can not update access token"
		//event.Source = corev1.EventSource{}
		event.InvolvedObject = corev1.ObjectReference{Namespace: secret.Namespace, Name: secret.Name, Kind: secret.Kind, APIVersion: secret.APIVersion}
		event.Type = "Error"
		//event.EventTime = metav1.NewTime(Now())
		event.LastTimestamp = metav1.NewTime(time.Now())

		err1 := r.Create(context.TODO(), event)

		log.Log.Error(err, msg)

		if err1 != nil {
			log.Log.Error(err1, "Event creation failed ")
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
