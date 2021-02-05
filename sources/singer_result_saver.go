package sources

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jitsucom/eventnative/drivers"
	"github.com/jitsucom/eventnative/events"
	"github.com/jitsucom/eventnative/logging"
	"github.com/jitsucom/eventnative/meta"
	"github.com/jitsucom/eventnative/metrics"
	"github.com/jitsucom/eventnative/singer"
	"github.com/jitsucom/eventnative/storages"
	"github.com/jitsucom/eventnative/timestamp"
	"github.com/jitsucom/eventnative/uuid"
	"strings"
)

type ResultSaver struct {
	identifier   string
	sourceId     string
	tap          string
	strLogger    *logging.SyncLogger
	destinations []storages.Storage
	metaStorage  meta.Storage
}

func NewResultSaver(identifier, sourceId, tap string, strLogger *logging.SyncLogger, destinations []storages.Storage, metaStorage meta.Storage) *ResultSaver {
	return &ResultSaver{
		identifier:   identifier,
		sourceId:     sourceId,
		tap:          tap,
		strLogger:    strLogger,
		destinations: destinations,
		metaStorage:  metaStorage,
	}
}

func (rs *ResultSaver) Consume(representation *singer.OutputRepresentation) error {
	for tableName, stream := range representation.Streams {
		rs.strLogger.Infof("[%s] Table [%s] key fields [%s] objects [%d]", rs.identifier, tableName, strings.Join(stream.KeyFields, ","), len(stream.Objects))

		for _, object := range stream.Objects {
			//enrich with system fields values
			object["src"] = "source"
			object[timestamp.Key] = timestamp.NowUTC()

			//calculate eventId from key fields or whole object
			var eventId string
			if len(stream.KeyFields) > 0 {
				eventId = uuid.GetKeysHash(object, stream.KeyFields)
			} else {
				eventId = uuid.GetHash(object)
			}
			events.EnrichWithEventId(object, eventId)
		}

		//Sync stream
		for _, storage := range rs.destinations {
			rowsCount, err := storage.SyncStore(stream.BatchHeader, stream.Objects, "")
			if err != nil {
				errMsg := fmt.Sprintf("[%s] Error storing %d source objects in [%s] destination: %v", rs.identifier, rowsCount, storage.Name(), err)
				metrics.ErrorSourceEvents(rs.sourceId, storage.Name(), rowsCount)
				metrics.ErrorObjects(rs.sourceId, rowsCount)
				return errors.New(errMsg)
			}

			metrics.SuccessSourceEvents(rs.sourceId, storage.Name(), rowsCount)
			metrics.SuccessObjects(rs.sourceId, rowsCount)
		}

		rs.strLogger.Infof("[%s] Synchronized successfully Table [%s] key fields [%s] objects [%d]", rs.identifier, tableName, strings.Join(stream.KeyFields, ","), len(stream.Objects))
	}

	stateJson, _ := json.Marshal(representation.State)

	err := rs.metaStorage.SaveSignature(rs.sourceId, rs.tap, drivers.ALL.String(), string(stateJson))
	if err != nil {
		errMsg := fmt.Sprintf("Unable to save source [%s] tap [%s] signature [%s]: %v", rs.sourceId, rs.tap, string(stateJson), err)
		logging.SystemError(errMsg)
		return errors.New(errMsg)
	}

	return nil
}