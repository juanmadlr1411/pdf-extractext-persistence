// Package mongo gestiona la conexión a MongoDB y el ownership de los índices
// de la colección de documentos del Persistence Service.
package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Database define el ciclo de vida de la conexión y el ownership de índices.
type Database interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	SetupIndexes(ctx context.Context) error
}

// MongoDBClient envuelve el cliente oficial de MongoDB junto con los nombres de
// database y colección de los que el servicio es dueño.
type MongoDBClient struct {
	uri        string
	client     *mongo.Client
	database   string
	collection string
}

// Client mantiene compatibilidad con el tipo anterior.
type Client = MongoDBClient

var _ Database = (*MongoDBClient)(nil)

// New crea un MongoDBClient sin conectar aún. La conexión se establece en Connect.
func New(uri, database, collection string) *MongoDBClient {
	return &MongoDBClient{
		uri:        uri,
		database:   database,
		collection: collection,
	}
}

// Connect establece la conexión con MongoDB y la verifica con un Ping.
func (c *MongoDBClient) Connect(ctx context.Context) error {
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(c.uri))
	if err != nil {
		return fmt.Errorf("no se pudo conectar a MongoDB: %w", err)
	}
	if err := mongoClient.Ping(ctx, nil); err != nil {
		return fmt.Errorf("MongoDB no responde al ping: %w", err)
	}
	c.client = mongoClient
	return nil
}

// Check implementa service.HealthChecker: hace Ping a MongoDB.
func (c *MongoDBClient) Check(ctx context.Context) error {
	if err := c.client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("MongoDB no responde al ping: %w", err)
	}
	return nil
}

// Disconnect cierra la conexión con MongoDB.
func (c *MongoDBClient) Disconnect(ctx context.Context) error {
	if err := c.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("no se pudo cerrar la conexión a MongoDB: %w", err)
	}
	return nil
}

// SetupIndexes garantiza el índice único sobre checksum (idempotente).
func (c *MongoDBClient) SetupIndexes(ctx context.Context) error {
	model := mongo.IndexModel{
		Keys:    bson.D{{Key: "checksum", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	if _, err := c.Collection().Indexes().CreateOne(ctx, model); err != nil {
		return fmt.Errorf("no se pudo crear el índice checksum_1: %w", err)
	}
	return nil
}

// Collection expone la colección gestionada por este cliente.
func (c *MongoDBClient) Collection() *mongo.Collection {
	return c.client.Database(c.database).Collection(c.collection)
}
