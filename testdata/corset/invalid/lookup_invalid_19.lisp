;;error:7:12-16:duplicate handle
(module m1)
(defcolumns (X :i16) (Y :i16) (one :i1 :constant 1))
(deflookup test ((one X)) ((one Y)))

(module m1)
(deflookup test ((one Y)) ((one X)))
