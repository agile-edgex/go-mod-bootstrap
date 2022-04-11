/*******************************************************************************
 * Copyright 2022 Intel Corp.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License
 * is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express
 * or implied. See the License for the specific language governing permissions and limitations under
 * the License.
 *******************************************************************************/

package metrics

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/edgexfoundry/go-mod-bootstrap/v2/bootstrap/interfaces"
	"github.com/edgexfoundry/go-mod-bootstrap/v2/config"
	"github.com/edgexfoundry/go-mod-messaging/v2/messaging"
	"github.com/edgexfoundry/go-mod-messaging/v2/pkg/types"

	"github.com/edgexfoundry/go-mod-core-contracts/v2/clients/logger"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/common"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/dtos"

	"github.com/hashicorp/go-multierror"
	gometrics "github.com/rcrowley/go-metrics"
)

const (
	serviceNameTagKey = "service"
	counterName       = "counter"
	gaugeName         = "gauge"
	gaugeFloat64Name  = "gauge-float64"
	timerName         = "timer"
)

type messageBusReporter struct {
	lc            logger.LoggingClient
	serviceName   string
	messageClient messaging.MessageClient
	config        *config.TelemetryInfo
}

// NewMessageBusReporter creates a new MessageBus reporter which reports metrics to the EdgeX MessageBus
func NewMessageBusReporter(lc logger.LoggingClient, serviceName string, messageClient messaging.MessageClient, config *config.TelemetryInfo) interfaces.MetricsReporter {
	reporter := &messageBusReporter{
		lc:            lc,
		serviceName:   serviceName,
		messageClient: messageClient,
		config:        config,
	}

	return reporter
}

// Report collects all the current metrics and reports them to the EdgeX MessageBus
// The approach here was adapted from https://github.com/vrischmann/go-metrics-influxdb
func (r *messageBusReporter) Report(registry gometrics.Registry, metricTags map[string]map[string]string) error {
	var errs error
	publishedCount := 0

	// Build the service tags each time we report since that can be changed in the Writable config
	serviceTags := buildMetricTags(r.config.Tags)
	serviceTags = append(serviceTags, dtos.MetricTag{
		Name:  serviceNameTagKey,
		Value: r.serviceName,
	})

	registry.Each(func(name string, item interface{}) {
		var nextMetric dtos.Metric
		var err error

		isEnabled := r.config.MetricEnabled(name)
		if !isEnabled {
			// This metric is not enable so do not report it.
			return
		}

		tags := append(serviceTags, buildMetricTags(metricTags[name])...)

		switch metric := item.(type) {
		case gometrics.Counter:
			snapshot := metric.Snapshot()
			fields := []dtos.MetricField{{Name: counterName, Value: snapshot.Count()}}
			nextMetric, err = dtos.NewMetric(name, fields, tags)

		case gometrics.Gauge:
			snapshot := metric.Snapshot()
			fields := []dtos.MetricField{{Name: gaugeName, Value: snapshot.Value()}}
			nextMetric, err = dtos.NewMetric(name, fields, tags)

		case gometrics.GaugeFloat64:
			snapshot := metric.Snapshot()
			fields := []dtos.MetricField{{Name: gaugeFloat64Name, Value: snapshot.Value()}}
			nextMetric, err = dtos.NewMetric(name, fields, tags)

		case gometrics.Timer:
			snapshot := metric.Snapshot()
			fields := []dtos.MetricField{
				{Name: timerName, Value: snapshot.Count()},
				{Name: "min", Value: snapshot.Min()},
				{Name: "max", Value: snapshot.Max()},
				{Name: "mean", Value: snapshot.Mean()},
				{Name: "stddev", Value: snapshot.StdDev()},
				{Name: "variance", Value: snapshot.Variance()},
			}
			nextMetric, err = dtos.NewMetric(name, fields, tags)

		default:
			errs = multierror.Append(errs, fmt.Errorf("metric type %T not supported", metric))
			return
		}

		if err != nil {
			err = fmt.Errorf("unable to create metric for '%s': %s", name, err.Error())
			errs = multierror.Append(errs, err)
			return
		}

		payload, err := json.Marshal(nextMetric)
		if err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to marshal metric '%s' to JSON: %s", nextMetric.Name, err.Error()))
			return
		}

		message := types.MessageEnvelope{
			CorrelationID: uuid.NewString(),
			Payload:       payload,
			ContentType:   common.ContentTypeJSON,
		}

		topic := fmt.Sprintf("%s/%s", r.baseTopic(), name)
		if err := r.messageClient.Publish(message, topic); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to publish metric '%s' to topic '%s': %s", name, topic, err.Error()))
			return
		} else {
			publishedCount++
		}
	})

	r.lc.Debugf("Publish %d metrics to the '%s' base topic", publishedCount, r.baseTopic())

	return errs
}

func (r *messageBusReporter) baseTopic() string {
	return fmt.Sprintf("%s/%s", r.config.PublishTopicPrefix, r.serviceName)
}

func buildMetricTags(tags map[string]string) []dtos.MetricTag {
	var metricTags []dtos.MetricTag

	for tagName, tagValue := range tags {
		metricTags = append(metricTags, dtos.MetricTag{
			Name:  tagName,
			Value: tagValue,
		})
	}

	return metricTags
}
