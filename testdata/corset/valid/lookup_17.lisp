(defcolumns (X :i16) (Y :i16) (one :i1 :constant 1))
;; fragmented lookup with single fragment.
(deflookup test ((one X)) ((one Y)))
