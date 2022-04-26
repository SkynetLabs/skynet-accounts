package database

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// Schema defines a mapping between a collection name and the indexes that
	// must exist for that collection.
	Schema = map[string][]mongo.IndexModel{
		collUsers: {
			{
				Keys:    bson.M{"sub": 1},
				Options: options.Index().SetName("sub_unique").SetUnique(true),
			},
		},
		collSkylinks: {
			{
				Keys:    bson.M{"skylink": 1},
				Options: options.Index().SetName("skylink_unique").SetUnique(true),
			},
		},
		collUploads: {
			{
				Keys:    bson.M{"user_id": 1},
				Options: options.Index().SetName("user_id"),
			},
			{
				Keys:    bson.M{"skylink_id": 1},
				Options: options.Index().SetName("skylink_id"),
			},
		},
		collDownloads: {
			{
				Keys:    bson.M{"user_id": 1},
				Options: options.Index().SetName("user_id"),
			},
			{
				Keys:    bson.M{"skylink_id": 1},
				Options: options.Index().SetName("skylink_id"),
			},
		},
		collEmails: {
			{
				Keys:    bson.M{"failed_attempts": 1},
				Options: options.Index().SetName("failed_attempts"),
			},
			{
				Keys:    bson.M{"locked_by": 1},
				Options: options.Index().SetName("locked_by"),
			},
			{
				Keys:    bson.M{"sent_at": 1},
				Options: options.Index().SetName("sent_at"),
			},
			{
				Keys:    bson.M{"sent_by": 1},
				Options: options.Index().SetName("sent_by"),
			},
		},
		collChallenges: {
			{
				Keys:    bson.M{"challenge": 1},
				Options: options.Index().SetName("challenge"),
			},
			{
				Keys:    bson.M{"type": 1},
				Options: options.Index().SetName("type"),
			},
			{
				Keys:    bson.M{"expires_at": 1},
				Options: options.Index().SetName("expires_at"),
			},
		},
		collUnconfirmedUserUpdates: {
			{
				Keys:    bson.M{"challenge_id": 1},
				Options: options.Index().SetName("challenge_id"),
			},
			{
				Keys:    bson.M{"expires_at": 1},
				Options: options.Index().SetName("expires_at"),
			},
		},
		collConfiguration: {
			{
				Keys:    bson.M{"key": 1},
				Options: options.Index().SetName("key_unique").SetUnique(true),
			},
		},
		collAPIKeys: {
			{
				Keys:    bson.M{"key": 1},
				Options: options.Index().SetName("key_unique").SetUnique(true),
			},
			{
				Keys:    bson.M{"user_id": 1},
				Options: options.Index().SetName("user_id"),
			},
		},
	}
)
